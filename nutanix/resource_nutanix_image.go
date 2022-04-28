package nutanix

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"

	v3 "github.com/terraform-providers/terraform-provider-nutanix/client/v3"
	"github.com/terraform-providers/terraform-provider-nutanix/utils"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

const (
	// ImageKind Represents kind of resource
	ImageKind = "image"
	// DELETED ...
	DELETED = "DELETED"
	// ERROR ..
	ERROR = "ERROR"
	// WAITING ...
	WAITING = "WAITING"
)

var (
	imageDelay      = 10 * time.Second
	imageMinTimeout = 3 * time.Second
)

func resourceNutanixImage() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceNutanixImageCreate,
		ReadContext:   resourceNutanixImageRead,
		UpdateContext: resourceNutanixImageUpdate,
		DeleteContext: resourceNutanixImageDelete,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		SchemaVersion: 1,
		StateUpgraders: []schema.StateUpgrader{
			{
				Type:    resourceNutanixImageInstanceResourceV0().CoreConfigSchema().ImpliedType(),
				Upgrade: resourceImageInstanceStateUpgradeV0,
				Version: 0,
			},
		},
		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(DEFAULTWAITTIMEOUT * time.Minute),
			Update: schema.DefaultTimeout(DEFAULTWAITTIMEOUT * time.Minute),
			Delete: schema.DefaultTimeout(DEFAULTWAITTIMEOUT * time.Minute),
		},
		Schema: map[string]*schema.Schema{
			"api_version": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"metadata": {
				Type:     schema.TypeMap,
				Computed: true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},
			"categories": categoriesSchema(),
			"owner_reference": {
				Type:     schema.TypeMap,
				Optional: true,
				Computed: true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},
			"project_reference": {
				Type:     schema.TypeMap,
				Optional: true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},
			"name": {
				Type:     schema.TypeString,
				Required: true,
			},
			"state": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"description": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"availability_zone_reference": {
				Type:     schema.TypeMap,
				Optional: true,
				Computed: true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},
			"cluster_uuid": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"cluster_name": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"retrieval_uri_list": {
				Type:     schema.TypeList,
				Computed: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
			},
			"cluster_references": {
				Type:     schema.TypeList,
				Computed: true,
				Optional: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"kind": {
							Type:     schema.TypeString,
							Optional: true,
							Computed: true,
						},
						"uuid": {
							Type:     schema.TypeString,
							Optional: true,
							Computed: true,
						},
						"name": {
							Type:     schema.TypeString,
							Optional: true,
							Computed: true,
						},
					},
				},
			},
			"current_cluster_reference_list": {
				Type:     schema.TypeList,
				Computed: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"kind": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"uuid": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"name": {
							Type:     schema.TypeString,
							Computed: true,
						},
					},
				},
			},
			"image_type": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"checksum": {
				Type:     schema.TypeMap,
				Optional: true,
				Computed: true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},
			"source_uri": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"source_path": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"version": {
				Type:     schema.TypeMap,
				Optional: true,
				Computed: true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},
			"architecture": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"size_bytes": {
				Type:     schema.TypeInt,
				Computed: true,
			},
		},
	}
}

func resourceNutanixImageCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	log.Printf("[DEBUG] Creating Image: %s", d.Get("name").(string))
	client := meta.(*Client)
	conn := client.API

	request := &v3.ImageIntentInput{}
	spec := &v3.Image{}
	metadata := &v3.Metadata{}
	image := &v3.ImageResources{}

	n, nok := d.GetOk("name")
	desc, descok := d.GetOk("description")

	_, iok := d.GetOk("source_uri")
	_, pok := d.GetOk("source_path")

	// if both path and uri are provided, return an error
	if iok && pok {
		return diag.Errorf("both source_uri and source_path provided")
	}

	// Read Arguments and set request values
	if !nok {
		return diag.Errorf("please provide the required attribute name")
	}

	if err := getMetadataAttributes(d, metadata, "image"); err != nil {
		return diag.FromErr(err)
	}

	if descok {
		spec.Description = utils.StringPtr(desc.(string))
	}

	if err := getImageResource(d, image); err != nil {
		return diag.FromErr(err)
	}

	spec.Name = utils.StringPtr(n.(string))
	spec.Resources = image

	request.Metadata = metadata
	request.Spec = spec

	// Make request to the API
	resp, err := conn.V3.CreateImage(request)
	if err != nil {
		return diag.Errorf("error creating Nutanix Image %s: %+v", utils.StringValue(spec.Name), err)
	}

	UUID := *resp.Metadata.UUID
	// set terraform state
	d.SetId(UUID)

	taskUUID := resp.Status.ExecutionContext.TaskUUID.(string)

	// Wait for the Image to be available
	stateConf := &resource.StateChangeConf{
		Pending:    []string{"QUEUED", "RUNNING"},
		Target:     []string{"SUCCEEDED"},
		Refresh:    taskStateRefreshFunc(conn, taskUUID),
		Timeout:    d.Timeout(schema.TimeoutCreate),
		Delay:      imageDelay,
		MinTimeout: imageMinTimeout,
	}

	if _, errw := stateConf.WaitForStateContext(ctx); errw != nil {
		delErr := resourceNutanixImageDelete(ctx, d, meta)
		if delErr != nil && delErr.HasError() {
			delErr = append(delErr, diag.Errorf("error waiting for image (%s) to delete in creation", d.Id())...)
			return delErr
		}
		d.SetId("")
		return diag.Errorf("error waiting for image (%s) to create: %s", UUID, errw)
	}

	// if we need to upload an image, we do it now
	if pok {
		path := d.Get("source_path")

		err = conn.V3.UploadImage(UUID, path.(string))
		if err != nil {
			delErr := resourceNutanixImageDelete(ctx, d, meta)
			if delErr != nil {
				return delErr
			}

			return diag.Errorf("failed uploading image: %s", err)
		}
	}

	return resourceNutanixImageRead(ctx, d, meta)
}

func resourceNutanixImageRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	log.Printf("[DEBUG] Reading Image: %s", d.Get("name").(string))

	// Get client connection
	conn := meta.(*Client).API
	uuid := d.Id()

	// Make request to the API
	resp, err := conn.V3.GetImage(uuid)
	if err != nil {
		if strings.Contains(fmt.Sprint(err), "ENTITY_NOT_FOUND") {
			d.SetId("")
			return nil
		}
		return diag.Errorf("error reading image UUID (%s) with error %s", uuid, err)
	}

	m, c := setRSEntityMetadata(resp.Metadata)

	if err = d.Set("metadata", m); err != nil {
		return diag.Errorf("error setting metadata for image UUID(%s), %s", d.Id(), err)
	}
	if err = d.Set("categories", c); err != nil {
		return diag.Errorf("error setting categories for image UUID(%s), %s", d.Id(), err)
	}

	if err = d.Set("owner_reference", flattenReferenceValues(resp.Metadata.OwnerReference)); err != nil {
		return diag.Errorf("error setting owner_reference for image UUID(%s), %s", d.Id(), err)
	}
	d.Set("api_version", utils.StringValue(resp.APIVersion))
	d.Set("name", utils.StringValue(resp.Status.Name))
	d.Set("description", utils.StringValue(resp.Status.Description))

	if err = d.Set("availability_zone_reference", flattenReferenceValues(resp.Status.AvailabilityZoneReference)); err != nil {
		return diag.Errorf("error setting owner_reference for image UUID(%s), %s", d.Id(), err)
	}
	if err = flattenClusterReference(resp.Status.ClusterReference, d); err != nil {
		return diag.Errorf("error setting cluster_uuid or cluster_name for image UUID(%s), %s", d.Id(), err)
	}

	if err = d.Set("state", resp.Status.State); err != nil {
		return diag.Errorf("error setting state for image UUID(%s), %s", d.Id(), err)
	}

	if err = d.Set("image_type", resp.Status.Resources.ImageType); err != nil {
		return diag.Errorf("error setting image_type for image UUID(%s), %s", d.Id(), err)
	}

	if err = d.Set("source_uri", resp.Status.Resources.SourceURI); err != nil {
		return diag.Errorf("error setting source_uri for image UUID(%s), %s", d.Id(), err)
	}

	if err = d.Set("size_bytes", resp.Status.Resources.SizeBytes); err != nil {
		return diag.Errorf("error setting size_bytes for image UUID(%s), %s", d.Id(), err)
	}

	checksum := make(map[string]string)
	if resp.Status.Resources.Checksum != nil {
		checksum["checksum_algorithm"] = utils.StringValue(resp.Status.Resources.Checksum.ChecksumAlgorithm)
		checksum["checksum_value"] = utils.StringValue(resp.Status.Resources.Checksum.ChecksumValue)
	}

	if err = d.Set("checksum", checksum); err != nil {
		return diag.Errorf("error setting checksum for image UUID(%s), %s", d.Id(), err)
	}

	version := make(map[string]string)
	if resp.Status.Resources.Version != nil {
		version["product_version"] = utils.StringValue(resp.Status.Resources.Version.ProductVersion)
		version["product_name"] = utils.StringValue(resp.Status.Resources.Version.ProductName)
	}

	if err = d.Set("version", version); err != nil {
		return diag.Errorf("error setting version for image UUID(%s), %s", d.Id(), err)
	}

	uriList := make([]string, 0, len(resp.Status.Resources.RetrievalURIList))
	for _, uri := range resp.Status.Resources.RetrievalURIList {
		uriList = append(uriList, utils.StringValue(uri))
	}

	if err = d.Set("retrieval_uri_list", uriList); err != nil {
		return diag.Errorf("error setting retrieval_uri_list for image UUID(%s), %s", d.Id(), err)
	}

	if err := d.Set("cluster_references", flattenArrayOfReferenceValues(resp.Status.Resources.InitialPlacementRefList)); err != nil {
		return diag.FromErr(err)
	}

	if err := d.Set("current_cluster_reference_list", flattenArrayOfReferenceValues(resp.Status.Resources.CurrentClusterReferenceList)); err != nil {
		return diag.FromErr(err)
	}

	return nil
}

func resourceNutanixImageUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*Client)
	conn := client.API

	// get state
	request := &v3.ImageIntentInput{}
	metadata := &v3.Metadata{}
	spec := &v3.Image{}
	res := &v3.ImageResources{}

	response, err := conn.V3.GetImage(d.Id())

	if err != nil {
		if strings.Contains(fmt.Sprint(err), "ENTITY_NOT_FOUND") {
			d.SetId("")
		}
		return diag.FromErr(err)
	}

	if response.Metadata != nil {
		metadata = response.Metadata
	}

	if response.Spec != nil {
		spec = response.Spec

		if response.Spec.Resources != nil {
			res = response.Spec.Resources
		}
	}

	if d.HasChange("categories") {
		metadata.Categories = expandCategories(d.Get("categories"))
	}

	if d.HasChange("owner_reference") {
		or := d.Get("owner_reference").(map[string]interface{})
		metadata.OwnerReference = validateRef(or)
	}

	if d.HasChange("project_reference") {
		pr := d.Get("project_reference").(map[string]interface{})
		metadata.ProjectReference = validateRef(pr)
	}

	if d.HasChange("name") {
		spec.Name = utils.StringPtr(d.Get("name").(string))
	}
	if d.HasChange("description") {
		spec.Description = utils.StringPtr(d.Get("description").(string))
	}

	if d.HasChange("source_uri") || d.HasChange("checksum") || d.HasChange("source_path") {
		if err := getImageResource(d, res); err != nil {
			return diag.FromErr(err)
		}
		spec.Resources = res
	}

	request.Metadata = metadata
	request.Spec = spec

	resp, errUpdate := conn.V3.UpdateImage(d.Id(), request)

	if errUpdate != nil {
		return diag.Errorf("error updating image(%s) %s", d.Id(), errUpdate)
	}

	taskUUID := resp.Status.ExecutionContext.TaskUUID.(string)

	// Wait for the Image to be available
	stateConf := &resource.StateChangeConf{
		Pending:    []string{"QUEUED", "RUNNING"},
		Target:     []string{"SUCCEEDED"},
		Refresh:    taskStateRefreshFunc(conn, taskUUID),
		Timeout:    d.Timeout(schema.TimeoutUpdate),
		Delay:      imageDelay,
		MinTimeout: imageMinTimeout,
	}

	if _, err := stateConf.WaitForStateContext(ctx); err != nil {
		delErr := resourceNutanixImageDelete(ctx, d, meta)
		if delErr != nil {
			delErr = append(delErr, diag.Errorf("error waiting for image (%s) to delete in update", d.Id())...)
			return delErr
		}
		uuid := d.Id()
		d.SetId("")
		return diag.Errorf("error waiting for image (%s) to update: %s", uuid, err)
	}

	return resourceNutanixImageRead(ctx, d, meta)
}

func resourceNutanixImageDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	log.Printf("[DEBUG] Deleting Image: %s", d.Get("name").(string))

	client := meta.(*Client)
	conn := client.API
	UUID := d.Id()

	resp, err := conn.V3.DeleteImage(UUID)
	if err != nil {
		if strings.Contains(fmt.Sprint(err), "ENTITY_NOT_FOUND") {
			d.SetId("")
		}
		return diag.FromErr(err)
	}

	taskUUID := resp.Status.ExecutionContext.TaskUUID.(string)

	// Wait for the Image to be available
	stateConf := &resource.StateChangeConf{
		Pending:    []string{"QUEUED", "RUNNING"},
		Target:     []string{"SUCCEEDED"},
		Refresh:    taskStateRefreshFunc(conn, taskUUID),
		Timeout:    d.Timeout(schema.TimeoutDelete),
		Delay:      imageDelay,
		MinTimeout: imageMinTimeout,
	}

	if _, err := stateConf.WaitForStateContext(ctx); err != nil {
		d.SetId("")
		return diag.Errorf("error waiting for image (%s) to delete: %s", d.Id(), err)
	}

	d.SetId("")
	return nil
}

func getImageResource(d *schema.ResourceData, image *v3.ImageResources) error {
	cs, csok := d.GetOk("checksum")
	checks := &v3.Checksum{}
	su, suok := d.GetOk("source_uri")
	sp, spok := d.GetOk("source_path")
	var furi string
	if suok {
		image.SourceURI = utils.StringPtr(su.(string))
		furi = su.(string)
	}
	if spok {
		furi = sp.(string)
	}
	if it, itok := d.GetOk("image_type"); itok {
		image.ImageType = utils.StringPtr(it.(string))
	} else {
		switch ext := filepath.Ext(furi); ext {
		case ".qcow2":
			image.ImageType = utils.StringPtr("DISK_IMAGE")
		case ".iso":
			image.ImageType = utils.StringPtr("ISO_IMAGE")
		default:
			// By default assuming the image to be raw disk image.
			image.ImageType = utils.StringPtr("DISK_IMAGE")
		}
		// set source uri
	}

	if csok {
		checksum := cs.(map[string]interface{})
		ca, caok := checksum["checksum_algorithm"]
		cv, cvok := checksum["checksum_value"]

		if caok {
			if ca.(string) == "" {
				return fmt.Errorf("'checksum_algorithm' is not given")
			}
			checks.ChecksumAlgorithm = utils.StringPtr(ca.(string))
		}
		if cvok {
			if cv.(string) == "" {
				return fmt.Errorf("'checksum_value' is not given")
			}
			checks.ChecksumValue = utils.StringPtr(cv.(string))
		}
		image.Checksum = checks
	}

	// List of clusters where image is requested to be placed at time of creation
	if refs, refsok := d.GetOk("cluster_references"); refsok && len(refs.([]interface{})) > 0 {
		image.InitialPlacementRefList = validateArrayRefValues(refs, "cluster")
	}

	return nil
}

func resourceImageInstanceStateUpgradeV0(ctx context.Context, is map[string]interface{}, meta interface{}) (map[string]interface{}, error) {
	log.Printf("[DEBUG] Entering resourceImageInstanceStateUpgradeV0")
	return resourceNutanixCategoriesMigrateState(is, meta)
}

func resourceNutanixImageInstanceResourceV0() *schema.Resource {
	return &schema.Resource{
		Schema: map[string]*schema.Schema{
			"api_version": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"metadata": {
				Type:     schema.TypeMap,
				Computed: true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},
			"categories": {
				Type:     schema.TypeMap,
				Optional: true,
				Computed: true,
			},
			"owner_reference": {
				Type:     schema.TypeMap,
				Optional: true,
				Computed: true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},
			"project_reference": {
				Type:     schema.TypeMap,
				Optional: true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},
			"name": {
				Type:     schema.TypeString,
				Required: true,
			},
			"state": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"description": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"availability_zone_reference": {
				Type:     schema.TypeMap,
				Optional: true,
				Computed: true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},
			"cluster_uuid": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"cluster_name": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"retrieval_uri_list": {
				Type:     schema.TypeList,
				Computed: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
			},
			"cluster_references": {
				Type:     schema.TypeList,
				Computed: true,
				Optional: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"kind": {
							Type:     schema.TypeString,
							Optional: true,
							Computed: true,
						},
						"uuid": {
							Type:     schema.TypeString,
							Optional: true,
							Computed: true,
						},
						"name": {
							Type:     schema.TypeString,
							Optional: true,
							Computed: true,
						},
					},
				},
			},
			"current_cluster_reference_list": {
				Type:     schema.TypeList,
				Computed: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"kind": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"uuid": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"name": {
							Type:     schema.TypeString,
							Computed: true,
						},
					},
				},
			},
			"image_type": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"checksum": {
				Type:     schema.TypeMap,
				Optional: true,
				Computed: true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},
			"source_uri": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"source_path": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"version": {
				Type:     schema.TypeMap,
				Optional: true,
				Computed: true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},
			"architecture": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"size_bytes": {
				Type:     schema.TypeInt,
				Computed: true,
			},
		},
	}
}
