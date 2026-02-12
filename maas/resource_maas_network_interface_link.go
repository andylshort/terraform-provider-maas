package maas

import (
	"context"
	"fmt"
	"strconv"

	"github.com/canonical/gomaasclient/client"
	"github.com/canonical/gomaasclient/entity"
	"github.com/canonical/gomaasclient/entity/node"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

func resourceMAASNetworkInterfaceLink() *schema.Resource {
	return &schema.Resource{
		Description:   "Provides a resource to manage network configuration on a network interface.",
		CreateContext: resourceNetworkInterfaceLinkCreate,
		ReadContext:   resourceNetworkInterfaceLinkRead,
		UpdateContext: resourceNetworkInterfaceLinkUpdate,
		DeleteContext: resourceNetworkInterfaceLinkDelete,

		Schema: map[string]*schema.Schema{
			"default_gateway": {
				Type:          schema.TypeBool,
				Optional:      true,
				Default:       false,
				ConflictsWith: []string{"device"},
				Description:   "Boolean value. When enabled, it sets the subnet gateway IP address as the default gateway for the machine the interface belongs to. This option can only be used with the `AUTO` and `STATIC` modes. Defaults to `false`.",
			},
			"device": {
				Type:         schema.TypeString,
				Optional:     true,
				ExactlyOneOf: []string{"machine", "device"},
				Description:  "The identifier (system ID, hostname, or FQDN) of the device with the network interface. Either `machine` or `device` must be provided.",
			},
			"ip_address": {
				Type:             schema.TypeString,
				Optional:         true,
				ForceNew:         true,
				Computed:         true,
				ValidateDiagFunc: validation.ToDiagFunc(validation.IsIPAddress),
				Description:      "Valid IP address (from the given subnet) to be configured on the network interface. Only used when `mode` is set to `STATIC`.",
			},
			"machine": {
				Type:         schema.TypeString,
				Optional:     true,
				ExactlyOneOf: []string{"machine", "device"},
				Description:  "The identifier (system ID, hostname, or FQDN) of the machine with the network interface. Either `machine` or `device` must be provided.",
			},
			"mode": {
				Type:             schema.TypeString,
				Optional:         true,
				ForceNew:         true,
				Default:          "AUTO",
				ValidateDiagFunc: validation.ToDiagFunc(validation.StringInSlice([]string{"AUTO", "DHCP", "STATIC", "LINK_UP"}, false)),
				Description:      "Connection mode to subnet. It defaults to `AUTO`. Valid options are:\n\t* `AUTO` - Random static IP address from the subnet.\n\t* `DHCP` - IP address from the DHCP on the given subnet.\n\t* `STATIC` - Use `ip_address` as static IP address.\n\t* `LINK_UP` - Bring the interface up only on the given subnet. No IP address will be assigned.",
			},
			"network_interface": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "The identifier (MAC address, name, or ID) of the network interface.",
			},
			"subnet": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "The identifier (CIDR or ID) of the subnet to be connected.",
			},
		},
	}
}

func resourceNetworkInterfaceLinkCreate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	client := meta.(*ClientConfig).Client

	systemID, err := getMachineOrDeviceSystemID(client, d)
	if err != nil {
		return diag.FromErr(err)
	}

	networkInterface, err := getNetworkInterface(client, systemID, d.Get("network_interface").(string))
	if err != nil {
		return diag.FromErr(err)
	}

	subnet, err := getSubnet(client, d.Get("subnet").(string))
	if err != nil {
		return diag.FromErr(err)
	}

	link, err := createNetworkInterfaceLink(client, systemID, networkInterface, getNetworkInterfaceLinkParams(d, subnet.ID))
	if err != nil {
		return diag.FromErr(err)
	}

	// Save the resource id
	d.SetId(fmt.Sprintf("%v", link.ID))

	return resourceNetworkInterfaceLinkRead(ctx, d, meta)
}

func resourceNetworkInterfaceLinkRead(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	client := meta.(*ClientConfig).Client

	// Get params for the read operation
	linkID, err := strconv.Atoi(d.Id())
	if err != nil {
		return diag.FromErr(err)
	}

	systemID, err := getMachineOrDeviceSystemID(client, d)
	if err != nil {
		return diag.FromErr(err)
	}

	networkInterface, err := getNetworkInterface(client, systemID, d.Get("network_interface").(string))
	if err != nil {
		return diag.FromErr(err)
	}

	// Get the network interface link
	link, err := getNetworkInterfaceLink(client, systemID, networkInterface.ID, linkID)
	if err != nil {
		return diag.FromErr(err)
	}

	// Set the Terraform state
	if err := d.Set("ip_address", link.IPAddress); err != nil {
		return diag.FromErr(err)
	}

	return nil
}

func resourceNetworkInterfaceLinkUpdate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	client := meta.(*ClientConfig).Client

	// Get params for the update operation
	linkID, err := strconv.Atoi(d.Id())
	if err != nil {
		return diag.FromErr(err)
	}

	systemID, err := getMachineOrDeviceSystemID(client, d)
	if err != nil {
		return diag.FromErr(err)
	}

	networkInterface, err := getNetworkInterface(client, systemID, d.Get("network_interface").(string))
	if err != nil {
		return diag.FromErr(err)
	}

	// Run update operation
	if _, err := client.Machine.ClearDefaultGateways(systemID); err != nil {
		return diag.FromErr(err)
	}

	if d.Get("default_gateway").(bool) {
		if _, err := client.NetworkInterface.SetDefaultGateway(systemID, networkInterface.ID, linkID); err != nil {
			return diag.FromErr(err)
		}
	}

	return resourceNetworkInterfaceLinkRead(ctx, d, meta)
}

func resourceNetworkInterfaceLinkDelete(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	client := meta.(*ClientConfig).Client

	// Get params for the delete operation
	linkID, err := strconv.Atoi(d.Id())
	if err != nil {
		return diag.FromErr(err)
	}

	systemID, err := getMachineOrDeviceSystemID(client, d)
	if err != nil {
		return diag.FromErr(err)
	}

	networkInterface, err := getNetworkInterface(client, systemID, d.Get("network_interface").(string))
	if err != nil {
		return diag.FromErr(err)
	}

	// Delete the network interface link
	if err := deleteNetworkInterfaceLink(client, systemID, networkInterface.ID, linkID); err != nil {
		return diag.FromErr(err)
	}

	return nil
}

func getNetworkInterfaceLinkParams(d *schema.ResourceData, subnetID int) *entity.NetworkInterfaceLinkParams {
	return &entity.NetworkInterfaceLinkParams{
		Subnet:         subnetID,
		Mode:           d.Get("mode").(string),
		DefaultGateway: d.Get("default_gateway").(bool),
		IPAddress:      d.Get("ip_address").(string),
	}
}

func createNetworkInterfaceLink(client *client.Client, machineSystemID string, networkInterface *entity.NetworkInterface, params *entity.NetworkInterfaceLinkParams) (*entity.NetworkInterfaceLink, error) {
	// Clear existing links
	for _, link := range networkInterface.Links {
		err := unlinkSubnet(client, machineSystemID, networkInterface.ID, link.ID)
		if err != nil {
			return nil, err
		}
	}

	// Create new link
	networkInterface, err := client.NetworkInterface.LinkSubnet(machineSystemID, networkInterface.ID, params)
	if err != nil {
		return nil, err
	}

	return &networkInterface.Links[0], nil
}

func getNetworkInterfaceLink(client *client.Client, machineSystemID string, networkInterfaceID int, linkID int) (*entity.NetworkInterfaceLink, error) {
	networkInterface, err := client.NetworkInterface.Get(machineSystemID, networkInterfaceID)
	if err != nil {
		return nil, err
	}

	for _, link := range networkInterface.Links {
		if link.ID == linkID {
			return &link, nil
		}
	}

	return nil, fmt.Errorf("cannot find link (%v) on the network interface (%v) from machine (%s)", linkID, networkInterfaceID, machineSystemID)
}

func deleteNetworkInterfaceLink(client *client.Client, machineSystemID string, networkInterfaceID int, linkID int) error {
	return unlinkSubnet(client, machineSystemID, networkInterfaceID, linkID)
}

func unlinkSubnet(client *client.Client, machineSystemID string, networkInterfaceID int, linkID int) error {
	// Interfaces may only be unlinked from subnets when the machine(s) they are attached to are in valid states.
	// Unlinking an interface when the machine is not in a valid state is not allowed and can result errors.
	// To address this, we introduce this handler whose job is to ensure that the machine is in a valid state before unlinking.
	//
	// Valid states include: New, Ready, Allocated, Broken. In other states we need to handle this operation differently,
	// for example in transitional states compared to non-transitional states (for instance, Deploying vs. Deployed).
	//
	// There are four scenarios to consider:
	// 1. The machine no longer exists. Unlinking should result in a no-op.
	// 2. The machine is in a valid state. Unlinking is allowed.
	// 3. The machine is in a transitional state. TBD.
	// 4. The machine is in a non-transitional state. TBD.

	// Obtain the state of the machine so we can ascertain how to handle proper unlinking
	machine, err := client.Machine.Get(machineSystemID)
	if err != nil {
		return nil //nolint:nilerr // The machine doesn't or no longer exists, so this is a no-op
	}

	switch machine.Status {
	// Valid states
	case node.StatusNew, node.StatusReady, node.StatusAllocated, node.StatusBroken:
		// This is the valid case where unlinking is straight-forward and allowed
		_, err = client.NetworkInterface.UnlinkSubnet(machineSystemID, networkInterfaceID, linkID)
		if err != nil {
			return err
		}

	// Transitional states
	case
		node.StatusCommissioning,
		node.StatusDeploying,
		node.StatusReleasing,
		node.StatusDiskErasing,
		node.StatusEnteringRescureMode,
		node.StatusExitingRescueMode,
		node.StatusTesting:
		eventLogMsg := fmt.Sprintf("Terraform requested machine %s be destroyed. Aborting current operation...", machine.SystemID)

		machine, err = client.Machine.Abort(machine.SystemID, eventLogMsg)
		if err != nil {
			return err
		}

		releaseParams := &entity.MachineReleaseParams{}

		_, err = client.Machine.Release(machine.SystemID, releaseParams)
		if err != nil {
			return err
		}

		_, err = client.NetworkInterface.UnlinkSubnet(machineSystemID, networkInterfaceID, linkID)
		if err != nil {
			return err
		}

	// Non-transitional states
	case
		node.StatusFailedCommissioning,
		node.StatusMissing,
		node.StatusReserved,
		node.StatusDeployed,
		node.StatusRetired,
		node.StatusFailedDeployment,
		node.StatusFailedReleasing,
		node.StatusFailedDiskErasing,
		node.StatusRescueMode,
		node.StatusFailedEnteringRescueMode,
		node.StatusFailedExitingRescueMode,
		node.StatusFailedTesting:
		releaseParams := &entity.MachineReleaseParams{}

		_, err = client.Machine.Release(machine.SystemID, releaseParams)
		if err != nil {
			return err
		}

		_, err = client.NetworkInterface.UnlinkSubnet(machineSystemID, networkInterfaceID, linkID)
		if err != nil {
			return err
		}

	default:
		// node.StatusDefault is left over
		return fmt.Errorf("cannot unlink subnet from machine in status %v", machine.Status)
	}

	return nil
}
