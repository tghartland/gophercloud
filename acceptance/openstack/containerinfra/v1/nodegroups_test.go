// +build acceptance containerinfra

package v1

import (
	"fmt"
	"testing"
	"time"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/acceptance/clients"
	"github.com/gophercloud/gophercloud/acceptance/tools"
	"github.com/gophercloud/gophercloud/openstack/containerinfra/v1/clusters"
	"github.com/gophercloud/gophercloud/openstack/containerinfra/v1/clustertemplates"
	"github.com/gophercloud/gophercloud/openstack/containerinfra/v1/nodegroups"
	th "github.com/gophercloud/gophercloud/testhelper"
)

func TestNodeGroupsCRUD(t *testing.T) {
	// API not available until Magnum train
	clients.SkipRelease(t, "stable/mitaka")
	clients.SkipRelease(t, "stable/newton")
	clients.SkipRelease(t, "stable/ocata")
	clients.SkipRelease(t, "stable/pike")
	clients.SkipRelease(t, "stable/queens")
	clients.SkipRelease(t, "stable/rocky")
	clients.SkipRelease(t, "stable/stein")

	client, err := clients.NewContainerInfraV1Client()
	th.AssertNoErr(t, err)

	client.Microversion = "1.9"

	clusterTemplate, err := createKubernetesClusterTemplate(t, client)
	th.AssertNoErr(t, err)
	defer DeleteClusterTemplate(t, client, clusterTemplate.UUID)

	clusterID, err := createKubernetesCluster(t, client, clusterTemplate.UUID)
	th.AssertNoErr(t, err)
	defer DeleteCluster(t, client, clusterID)

	t.Run("list", func(t *testing.T) { testNodeGroupsList(t, client, clusterID) })
	t.Run("listone-get", func(t *testing.T) { testNodeGroupGet(t, client, clusterID) })
}

func testNodeGroupsList(t *testing.T, client *gophercloud.ServiceClient, clusterID string) {
	allPages, err := nodegroups.List(client, clusterID, nil).AllPages()
	th.AssertNoErr(t, err)

	allNodeGroups, err := nodegroups.ExtractNodeGroups(allPages)
	th.AssertNoErr(t, err)

	// By default two node groups should be created
	th.AssertEquals(t, 2, len(allNodeGroups))
}

func testNodeGroupGet(t *testing.T, client *gophercloud.ServiceClient, clusterID string) {
	listOpts := nodegroups.ListOpts{
		Role: "worker",
	}
	allPages, err := nodegroups.List(client, clusterID, listOpts).AllPages()
	th.AssertNoErr(t, err)

	allNodeGroups, err := nodegroups.ExtractNodeGroups(allPages)
	th.AssertNoErr(t, err)

	// Should be one worker node group
	th.AssertEquals(t, 1, len(allNodeGroups))

	ngID := allNodeGroups[0].UUID

	ng, err := nodegroups.Get(client, clusterID, ngID).Extract()
	th.AssertNoErr(t, err)

	// Should have got the same node group as from the list
	th.AssertEquals(t, ngID, ng.UUID)
	th.AssertEquals(t, "worker", ng.Role)
}

func createKubernetesClusterTemplate(t *testing.T, client *gophercloud.ServiceClient) (*clustertemplates.ClusterTemplate, error) {
	choices, err := clients.AcceptanceTestChoicesFromEnv()
	if err != nil {
		return nil, err
	}

	name := tools.RandomString("TESTACC-", 8)
	t.Logf("Attempting to create cluster template: %s", name)

	boolFalse := false
	createOpts := clustertemplates.CreateOpts{
		COE:                 "kubernetes",
		DNSNameServer:       "8.8.8.8",
		DockerStorageDriver: "overlay2",
		ExternalNetworkID:   choices.ExternalNetworkID,
		FlavorID:            choices.FlavorID,
		FloatingIPEnabled:   &boolFalse,
		ImageID:             choices.MagnumImageID,
		MasterFlavorID:      choices.FlavorID,
		MasterLBEnabled:     &boolFalse,
		Name:                name,
		Public:              &boolFalse,
		RegistryEnabled:     &boolFalse,
		ServerType:          "vm",
	}

	res := clustertemplates.Create(client, createOpts)
	clusterTemplate, err := res.Extract()
	if err != nil {
		return nil, err
	}

	t.Logf("Successfully created cluster template: %s", clusterTemplate.Name)

	tools.PrintResource(t, clusterTemplate)
	tools.PrintResource(t, clusterTemplate.CreatedAt)

	th.AssertEquals(t, name, clusterTemplate.Name)
	th.AssertEquals(t, choices.ExternalNetworkID, clusterTemplate.ExternalNetworkID)
	th.AssertEquals(t, choices.MagnumImageID, clusterTemplate.ImageID)

	return clusterTemplate, nil
}

func createKubernetesCluster(t *testing.T, client *gophercloud.ServiceClient, clusterTemplateID string) (string, error) {
	clusterName := tools.RandomString("TESTACC-", 8)
	t.Logf("Attempting to create cluster: %s using template %s", clusterName, clusterTemplateID)

	choices, err := clients.AcceptanceTestChoicesFromEnv()
	if err != nil {
		return "", err
	}

	masterCount := 1
	nodeCount := 1
	createTimeout := 900
	createOpts := clusters.CreateOpts{
		ClusterTemplateID: clusterTemplateID,
		CreateTimeout:     &createTimeout,
		FlavorID:          choices.FlavorID,
		Keypair:           choices.MagnumKeypair,
		Labels:            map[string]string{},
		MasterCount:       &masterCount,
		MasterFlavorID:    choices.FlavorID,
		Name:              clusterName,
		NodeCount:         &nodeCount,
	}

	createResult := clusters.Create(client, createOpts)
	th.AssertNoErr(t, createResult.Err)
	if len(createResult.Header["X-Openstack-Request-Id"]) > 0 {
		t.Logf("Cluster Create Request ID: %s", createResult.Header["X-Openstack-Request-Id"][0])
	}

	clusterID, err := createResult.Extract()
	if err != nil {
		return "", err
	}

	t.Logf("Cluster created: %+v", clusterID)

	clusterReady := false
	st := time.Now()
	for time.Since(st) < 15*time.Minute {
		time.Sleep(10 * time.Second)
		cluster, err := clusters.Get(client, clusterID).Extract()
		if err != nil {
			return "", fmt.Errorf("Error checking cluster status: %v", err)
		}
		if cluster.Status == "CREATE_COMPLETE" {
			clusterReady = true
			break
		}
	}

	if !clusterReady {
		return "", fmt.Errorf("Timing out waiting for cluster CREATE_COMPLETE status")
	}

	t.Logf("Successfully created cluster: %s id: %s", clusterName, clusterID)
	return clusterID, nil
}
