package azurerm

import (
	"fmt"
	"log"

	"github.com/Azure/azure-sdk-for-go/services/preview/operationalinsights/mgmt/2015-11-01-preview/operationalinsights"
	"github.com/hashicorp/terraform/helper/schema"
	"github.com/hashicorp/terraform/helper/validation"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/helpers/azure"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/helpers/suppress"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/helpers/tf"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/utils"
)

/*
TODO: refactor this:

 * resource_group_name/workspace_name can become case-sensitive
 * linked_service_properties should be a list / removed in favour of the top level element?
 * we can remove `workspace` from the resource name?
*/
func resourceArmLogAnalyticsWorkspaceLinkedService() *schema.Resource {
	return &schema.Resource{
		Create: resourceArmLogAnalyticsWorkspaceLinkedServiceCreateUpdate,
		Read:   resourceArmLogAnalyticsWorkspaceLinkedServiceRead,
		Update: resourceArmLogAnalyticsWorkspaceLinkedServiceCreateUpdate,
		Delete: resourceArmLogAnalyticsWorkspaceLinkedServiceDelete,

		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},

		Schema: map[string]*schema.Schema{
			"resource_group_name": resourceGroupNameDiffSuppressSchema(),

			"workspace_name": {
				Type:             schema.TypeString,
				Required:         true,
				ForceNew:         true,
				DiffSuppressFunc: suppress.CaseDifference,
				ValidateFunc:     validateAzureRmLogAnalyticsWorkspaceName,
			},

			"linked_service_name": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
				Default:  "automation",
				ValidateFunc: validation.StringInSlice([]string{
					"automation",
				}, false),
			},

			"linked_service_properties": {
				Type:     schema.TypeMap,
				Required: true,
				ForceNew: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"resource_id": {
							Type:         schema.TypeString,
							Required:     true,
							ForceNew:     true,
							ValidateFunc: azure.ValidateResourceID,
						},
					},
				},
			},

			// Exported properties
			"name": {
				Type:     schema.TypeString,
				Computed: true,
			},

			"tags": tagsSchema(),
		},
	}
}

func resourceArmLogAnalyticsWorkspaceLinkedServiceCreateUpdate(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*ArmClient).linkedServicesClient
	ctx := meta.(*ArmClient).StopContext

	log.Printf("[INFO] preparing arguments for AzureRM Log Analytics Linked Services creation.")

	resGroup := d.Get("resource_group_name").(string)
	workspaceName := d.Get("workspace_name").(string)
	lsName := d.Get("linked_service_name").(string)

	if requireResourcesToBeImported && d.IsNewResource() {
		existing, err := client.Get(ctx, resGroup, workspaceName, lsName)
		if err != nil {
			if !utils.ResponseWasNotFound(existing.Response) {
				return fmt.Errorf("Error checking for presence of existing Linked Service %q (Workspace %q / Resource Group %q): %s", lsName, workspaceName, resGroup, err)
			}
		}

		if existing.ID != nil && *existing.ID != "" {
			return tf.ImportAsExistsError("azurerm_log_analytics_workspace_linked_service", *existing.ID)
		}
	}

	props := d.Get("linked_service_properties").(map[string]interface{})
	resourceID := props["resource_id"].(string)

	tags := d.Get("tags").(map[string]interface{})

	parameters := operationalinsights.LinkedService{
		Tags: expandTags(tags),
		LinkedServiceProperties: &operationalinsights.LinkedServiceProperties{
			ResourceID: &resourceID,
		},
	}

	if _, err := client.CreateOrUpdate(ctx, resGroup, workspaceName, lsName, parameters); err != nil {
		return fmt.Errorf("Error creating Linked Service %q (Workspace %q / Resource Group %q): %+v", lsName, workspaceName, resGroup, err)
	}

	read, err := client.Get(ctx, resGroup, workspaceName, lsName)
	if err != nil {
		return fmt.Errorf("Error retrieving Linked Service %q (Worksppce %q / Resource Group %q): %+v", lsName, workspaceName, resGroup, err)
	}
	if read.ID == nil {
		return fmt.Errorf("Cannot read Linked Service %q (Workspace %q / Resource Group %q) ID", lsName, workspaceName, resGroup)
	}

	d.SetId(*read.ID)

	return resourceArmLogAnalyticsWorkspaceLinkedServiceRead(d, meta)
}

func resourceArmLogAnalyticsWorkspaceLinkedServiceRead(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*ArmClient).linkedServicesClient
	ctx := meta.(*ArmClient).StopContext

	id, err := parseAzureResourceID(d.Id())
	if err != nil {
		return err
	}

	resGroup := id.ResourceGroup
	workspaceName := id.Path["workspaces"]
	lsName := id.Path["linkedServices"]

	resp, err := client.Get(ctx, resGroup, workspaceName, lsName)
	if err != nil {
		if utils.ResponseWasNotFound(resp.Response) {
			d.SetId("")
			return nil
		}
		return fmt.Errorf("Error making Read request on AzureRM Log Analytics Linked Service '%s': %+v", lsName, err)
	}
	if resp.ID == nil {
		d.SetId("")
		return nil
	}

	d.Set("name", resp.Name)
	d.Set("resource_group_name", resGroup)
	d.Set("workspace_name", workspaceName)
	d.Set("linked_service_name", lsName)

	linkedServiceProperties := flattenLogAnalyticsWorkspaceLinkedServiceProperties(resp.LinkedServiceProperties)
	if err := d.Set("linked_service_properties", linkedServiceProperties); err != nil {
		return fmt.Errorf("Error setting `linked_service_properties`: %+v", err)
	}

	flattenAndSetTags(d, resp.Tags)
	return nil
}

func resourceArmLogAnalyticsWorkspaceLinkedServiceDelete(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*ArmClient).linkedServicesClient
	ctx := meta.(*ArmClient).StopContext

	id, err := parseAzureResourceID(d.Id())
	if err != nil {
		return err
	}

	resGroup := id.ResourceGroup
	workspaceName := id.Path["workspaces"]
	lsName := id.Path["linkedServices"]

	resp, err := client.Delete(ctx, resGroup, workspaceName, lsName)
	if err != nil {
		if utils.ResponseWasNotFound(resp) {
			return nil
		}

		return fmt.Errorf("Error deleting Linked Service %q (Workspace %q / Resource Group %q): %+v", lsName, workspaceName, resGroup, err)
	}

	return nil
}

func flattenLogAnalyticsWorkspaceLinkedServiceProperties(input *operationalinsights.LinkedServiceProperties) interface{} {
	if input == nil {
		return []interface{}{}
	}

	properties := make(map[string]interface{})

	// resource id linked service
	if resourceID := input.ResourceID; resourceID != nil {
		properties["resource_id"] = interface{}(*resourceID)
	}

	return interface{}(properties)
}
