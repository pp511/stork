//go:build integrationtest
// +build integrationtest

package integrationtest

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strconv"
	"strings"
	"testing"
	"time"

	snapv1 "github.com/kubernetes-incubator/external-storage/snapshot/pkg/apis/crd/v1"
	storkdriver "github.com/libopenstorage/stork/drivers/volume"
	_ "github.com/libopenstorage/stork/drivers/volume/aws"
	_ "github.com/libopenstorage/stork/drivers/volume/azure"
	_ "github.com/libopenstorage/stork/drivers/volume/csi"
	_ "github.com/libopenstorage/stork/drivers/volume/gcp"
	_ "github.com/libopenstorage/stork/drivers/volume/linstor"
	_ "github.com/libopenstorage/stork/drivers/volume/portworx"
	storkv1 "github.com/libopenstorage/stork/pkg/apis/stork/v1alpha1"
	"github.com/libopenstorage/stork/pkg/schedule"
	"github.com/libopenstorage/stork/pkg/storkctl"
	"github.com/libopenstorage/stork/pkg/version"
	"github.com/portworx/sched-ops/k8s/apps"
	"github.com/portworx/sched-ops/k8s/core"
	"github.com/portworx/sched-ops/k8s/dynamic"
	"github.com/portworx/sched-ops/k8s/externalstorage"
	"github.com/portworx/sched-ops/k8s/openshift"
	"github.com/portworx/sched-ops/k8s/rbac"
	"github.com/portworx/sched-ops/k8s/storage"
	"github.com/portworx/sched-ops/k8s/stork"
	"github.com/portworx/torpedo/drivers/node"
	_ "github.com/portworx/torpedo/drivers/node/ssh"
	"github.com/portworx/torpedo/drivers/objectstore"
	"github.com/portworx/torpedo/drivers/scheduler"
	_ "github.com/portworx/torpedo/drivers/scheduler/k8s"
	"github.com/portworx/torpedo/drivers/volume"
	_ "github.com/portworx/torpedo/drivers/volume/aws"
	_ "github.com/portworx/torpedo/drivers/volume/azure"
	_ "github.com/portworx/torpedo/drivers/volume/gce"
	_ "github.com/portworx/torpedo/drivers/volume/generic_csi"
	_ "github.com/portworx/torpedo/drivers/volume/linstor"
	_ "github.com/portworx/torpedo/drivers/volume/portworx"
	"github.com/sirupsen/logrus"
	"github.com/skyrings/skyring-common/tools/uuid"
	"github.com/stretchr/testify/require"
	appsapi "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	storageapi "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	nodeDriverName              = "ssh"
	cmName                      = "stork-version"
	defaultAdminNamespace       = "kube-system"
	schedulerDriverName         = "k8s"
	remotePairName              = "remoteclusterpair"
	srcConfig                   = "sourceconfigmap"
	destConfig                  = "destinationconfigmap"
	specDir                     = "./specs"
	tempDir                     = "/tmp"
	defaultClusterPairDir       = "cluster-pair"
	bidirectionalClusterPairDir = "bidirectional-cluster-pair"
	pairFileName                = "cluster-pair.yaml"
	remoteFilePath              = "/tmp/kubeconfig"
	configMapSyncWaitTime       = 3 * time.Second
	defaultSchedulerName        = "default-scheduler"
	bucketPrefix                = "stork-test"
	adminTokenSecretName        = "px-admin-token"

	// TODO: Figure out a way to communicate with PX nodes from other cluster
	nodeScore   = 100
	rackScore   = 50
	zoneScore   = 25
	regionScore = 10

	defaultWaitTimeout       time.Duration = 10 * time.Minute
	clusterDomainWaitTimeout time.Duration = 10 * time.Minute
	groupSnapshotWaitTimeout time.Duration = 15 * time.Minute
	defaultWaitInterval      time.Duration = 10 * time.Second
	backupWaitInterval       time.Duration = 2 * time.Second

	enableClusterDomainTests = "ENABLE_CLUSTER_DOMAIN_TESTS"
	storageProvisioner       = "STORAGE_PROVISIONER"
	authSecretConfigMap      = "AUTH_SECRET_CONFIGMAP"
	backupPathVar            = "BACKUP_LOCATION_PATH"
	externalTestCluster      = "EXTERNAL_TEST_CLUSTER"
	cloudDeletionValidation  = "CLOUD_DELETION_VALIDATION"

	tokenKey    = "token"
	clusterIP   = "ip"
	clusterPort = "port"
)

var nodeDriver node.Driver
var schedulerDriver scheduler.Driver
var volumeDriver volume.Driver

var storkVolumeDriver storkdriver.Driver
var objectStoreDriver objectstore.Driver

var snapshotScaleCount int
var migrationScaleCount int
var backupScaleCount int
var authToken string
var authTokenConfigMap string
var volumeDriverName string
var schedulerName string
var backupLocationPath string
var genericCsiConfigMap string
var externalTest bool
var storkVersionCheck bool
var cloudDeletionValidate bool

func TestSnapshot(t *testing.T) {
	t.Run("testSnapshot", testSnapshot)
	t.Run("testSnapshotRestore", testSnapshotRestore)
	t.Run("testWebhook", testWebhook)
}

func TestStorkCbt(t *testing.T) {
	t.Run("deploymentTest", deploymentMigrationTest)
	t.Run("testMigrationFailoverFailback", testMigrationFailoverFailback)
	t.Run("stopDriverTest", stopDriverTest)
	t.Run("simpleSnapshotTest", simpleSnapshotTest)
	t.Run("pvcOwnershipTest", pvcOwnershipTest)
}

func TestStorkCbtBackup(t *testing.T) {
	setDefaultsForBackup(t)

	logrus.Infof("Using stork volume driver: %s", volumeDriverName)
	logrus.Infof("Backup path being used: %s", backupLocationPath)

	err := setSourceKubeConfig()
	require.NoError(t, err, "failed to set kubeconfig to source cluster: %v", err)
	t.Run("applicationBackupRestoreTest", applicationBackupRestoreTest)
}

// TODO: Take driver name from input
// TODO: Parse storageclass specs based on driver name
func setup() error {
	var err error

	externalTest, err = strconv.ParseBool(os.Getenv(externalTestCluster))
	if err == nil {
		logrus.Infof("Three cluster config mode has been activated for test: %t", externalTest)
	}

	err = setSourceKubeConfig()
	if err != nil {
		return fmt.Errorf("setting kubeconfig to source failed in setup: %v", err)
	}

	logrus.Infof("Using stork volume driver: %s", volumeDriverName)
	provisioner := os.Getenv(storageProvisioner)
	backupLocationPath = addTimestampSuffix(os.Getenv(backupPathVar))
	if storkVolumeDriver, err = storkdriver.Get(volumeDriverName); err != nil {
		return fmt.Errorf("Error getting stork volume driver %s: %v", volumeDriverName, err)
	}

	if err = storkVolumeDriver.Init(nil); err != nil {
		return fmt.Errorf("Error initializing stork volume driver %v: %v", volumeDriverName, err)
	}

	if nodeDriver, err = node.Get(nodeDriverName); err != nil {
		return fmt.Errorf("Error getting node driver %v: %v", nodeDriverName, err)
	}

	if err = nodeDriver.Init(node.InitOptions{}); err != nil {
		return fmt.Errorf("Error initializing node driver %v: %v", nodeDriverName, err)
	}

	if schedulerDriver, err = scheduler.Get(schedulerDriverName); err != nil {
		return fmt.Errorf("Error getting scheduler driver %v: %v", schedulerDriverName, err)
	}

	if volumeDriver, err = volume.Get(volumeDriverName); err != nil {
		return fmt.Errorf("Error getting volume driver %v: %v", volumeDriverName, err)
	}

	cloudDeletionValidate, err = strconv.ParseBool(os.Getenv(cloudDeletionValidation))
	if err == nil {
		logrus.Infof("Three cluster config mode has been activated for test: %t", externalTest)
	}
	if objectStoreDriver, err = objectstore.Get(); err != nil {
		return fmt.Errorf("Error getting volume driver %v: %v", volumeDriverName, err)
	}

	authTokenConfigMap = os.Getenv(authSecretConfigMap)
	if authTokenConfigMap != "" {
		if authToken, err = schedulerDriver.GetTokenFromConfigMap(authTokenConfigMap); err != nil {
			return fmt.Errorf("Failed to get config map for token when running on an auth-enabled cluster %v", err)
		}
		logrus.Infof("Auth token used for initializing scheduler/volume driver: %s ", authToken)

	}
	logrus.Infof("Using provisioner: %s", provisioner)
	var customAppConfig map[string]scheduler.AppConfig
	// For non-PX backups use default-scheduler in apps instead of stork
	if provisioner != "pxd" {
		schedulerName = defaultSchedulerName
	}
	if err = schedulerDriver.Init(scheduler.InitOptions{SpecDir: "specs",
		VolDriverName:       volumeDriverName,
		NodeDriverName:      nodeDriverName,
		SecretConfigMapName: authTokenConfigMap,
		CustomAppConfig:     customAppConfig,
	}); err != nil {
		return fmt.Errorf("Error initializing scheduler driver %v: %v", schedulerDriverName, err)
	}

	if err = volumeDriver.Init(schedulerDriverName, nodeDriverName, authToken, provisioner, genericCsiConfigMap); err != nil {
		return fmt.Errorf("Error initializing volume driver %v: %v", volumeDriverName, err)
	}
	cm, err := core.Instance().GetConfigMap(cmName, defaultAdminNamespace)
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("Unable to get stork version configmap: %v", err)
	}
	if cm != nil && storkVersionCheck == true {
		ver, ok := cm.Data["version"]
		if !ok {
			return fmt.Errorf("stork version not found in configmap: %s", cmName)
		}
		if getStorkVersion(ver) != getStorkVersion(version.Version) {
			return fmt.Errorf("stork version mismatch, found: %s, expected: %s", getStorkVersion(ver), getStorkVersion(version.Version))
		}
	}
	return nil
}

func generateInstanceID(t *testing.T, testName string) string {
	id, err := uuid.New()
	require.NoError(t, err, "Error generating uuid for task")
	return testName + "-" + id.String()
}

func destroyAndWait(t *testing.T, ctxs []*scheduler.Context) {
	for _, ctx := range ctxs {
		err := schedulerDriver.Destroy(ctx, nil)
		require.NoError(t, err, "Error destroying ctx: %+v", ctx)
	}
	for _, ctx := range ctxs {
		err := schedulerDriver.WaitForDestroy(ctx, defaultWaitTimeout)
		require.NoError(t, err, "Error waiting for destroy of ctx: %+v", ctx)
		_, err = schedulerDriver.DeleteVolumes(ctx, nil)
		require.NoError(t, err, "Error deleting volumes in ctx: %+v", ctx)
	}
}

func getVolumeNames(t *testing.T, ctx *scheduler.Context) []string {
	volumeParams, err := schedulerDriver.GetVolumeParameters(ctx)
	require.NoError(t, err, "Error getting volume Parameters")

	var volumes []string
	for vol := range volumeParams {
		volumes = append(volumes, vol)
	}
	return volumes
}

func verifyScheduledNode(t *testing.T, appNode node.Node, volumes []string) {
	if externalTest {
		// TODO: Figure out a way to communicate with PX nodes from other cluster
		return
	}
	if schedulerName == defaultSchedulerName {
		return
	}

	driverNodes, err := storkVolumeDriver.GetNodes()
	require.NoError(t, err, "Error getting nodes from stork driver")

	found := false
	for _, dNode := range driverNodes {
		if dNode.Hostname == appNode.Name {
			found = true
			break
		}
		for _, address := range appNode.Addresses {
			for _, ip := range dNode.IPs {
				if ip == address {
					dNode.Hostname = appNode.Name
					found = true
					break
				}
			}
			if found {
				break
			}
		}
	}
	require.Equal(t, true, found, "Scheduled node not found in driver node list. DriverNodes: %v ScheduledNode: %v", driverNodes, appNode)

	scores := make(map[string]int)
	idMap := make(map[string]*storkdriver.NodeInfo)
	rackMap := make(map[string][]string)
	zoneMap := make(map[string][]string)
	regionMap := make(map[string][]string)
	for _, dNode := range driverNodes {
		scores[dNode.Hostname] = 0
		idMap[dNode.StorageID] = dNode
		if dNode.Status == storkdriver.NodeOnline {
			if dNode.Rack != "" {
				rackMap[dNode.Rack] = append(rackMap[dNode.Rack], dNode.Hostname)
			}
			if dNode.Zone != "" {
				zoneMap[dNode.Zone] = append(rackMap[dNode.Zone], dNode.Hostname)
			}
			if dNode.Region != "" {
				regionMap[dNode.Region] = append(rackMap[dNode.Region], dNode.Hostname)
			}
		}
	}

	// Calculate scores for each node
	for _, vol := range volumes {
		volInfo, err := storkVolumeDriver.InspectVolume(vol)
		require.NoError(t, err, "Error inspecting volume %v", vol)

		for _, dataNode := range volInfo.DataNodes {
			hostname := idMap[dataNode].Hostname
			scores[hostname] += nodeScore

			if idMap[dataNode].Rack != "" {
				for _, node := range rackMap[idMap[dataNode].Rack] {
					if dataNode != node {
						scores[node] += rackScore
					}
				}
			}
			if idMap[dataNode].Zone != "" {
				for _, node := range zoneMap[idMap[dataNode].Zone] {
					if dataNode != node {
						scores[node] += zoneScore
					}
				}
			}
			if idMap[dataNode].Rack != "" {
				for _, node := range regionMap[idMap[dataNode].Region] {
					if dataNode != node {
						scores[node] += regionScore
					}
				}
			}
		}
	}

	highScore := 0
	for _, score := range scores {
		if score > highScore {
			highScore = score
		}
	}

	logrus.Infof("Scores: %v", scores)
	require.Equal(t, highScore, scores[appNode.Name], "Scheduled node does not have the highest score")
}

// write kubbeconfig file to /tmp/kubeconfig
func dumpRemoteKubeConfig(configObject string) error {
	cm, err := core.Instance().GetConfigMap(configObject, "kube-system")
	if err != nil {
		logrus.Errorf("Error reading config map: %v", err)
		return err
	}
	config := cm.Data["kubeconfig"]
	if len(config) == 0 {
		configErr := "Error reading kubeconfig: found empty remoteConfig in config map"
		return fmt.Errorf(configErr)
	}
	// dump to remoteFilePath
	return ioutil.WriteFile(remoteFilePath, []byte(config), 0644)
}

func dumpKubeConfigPath(configObject string, path string) error {
	cm, err := core.Instance().GetConfigMap(configObject, "kube-system")
	if err != nil {
		logrus.Errorf("Error reading config map: %v", err)
		return err
	}
	config := cm.Data["kubeconfig"]
	if len(config) == 0 {
		configErr := "Error reading kubeconfig: found empty remoteConfig in config map"
		return fmt.Errorf(configErr)
	}
	// dump to remoteFilePath
	return ioutil.WriteFile(path, []byte(config), 0644)
}

func setRemoteConfig(kubeConfig string) error {

	var config *rest.Config
	var err error

	if kubeConfig == "" {
		config = nil
	} else {
		config, err = clientcmd.BuildConfigFromFlags("", kubeConfig)
		if err != nil {
			return err
		}
	}

	k8sOps := core.Instance()
	k8sOps.SetConfig(config)

	storkOps := stork.Instance()
	storkOps.SetConfig(config)

	appsOps := apps.Instance()
	appsOps.SetConfig(config)

	storageOps := storage.Instance()
	storageOps.SetConfig(config)

	dynamicOps := dynamic.Instance()
	dynamicOps.SetConfig(config)

	extOps := externalstorage.Instance()
	extOps.SetConfig(config)

	ocpOps := openshift.Instance()
	ocpOps.SetConfig(config)

	rbacOps := rbac.Instance()
	rbacOps.SetConfig(config)
	return nil
}

func setSourceKubeConfig() error {
	// setting kubeconfig to default cluster first since all configmaps are created there
	err := setRemoteConfig("")
	if err != nil {
		return fmt.Errorf("setting kubeconfig to default failed: %v", err)
	}

	// Change kubeconfig to source cluster
	err = dumpRemoteKubeConfig(srcConfig)
	if err != nil {
		return fmt.Errorf("unable to dump remote config while setting source config: %v", err)
	}

	err = setRemoteConfig(remoteFilePath)
	if err != nil {
		return fmt.Errorf("unable to set source config: %v", err)
	}
	return nil
}

func setDestinationKubeConfig() error {
	// setting kubeconfig to default cluster first since all configmaps are created there
	err := setRemoteConfig("")
	if err != nil {
		return fmt.Errorf("unable to set source config: %v", err)
	}

	// Change kubeconfig to source cluster
	err = dumpRemoteKubeConfig(destConfig)
	if err != nil {
		return fmt.Errorf("unable to dump remote config while setting source config: %v", err)
	}

	err = setRemoteConfig(remoteFilePath)
	if err != nil {
		return fmt.Errorf("unable to set source config: %v", err)
	}
	return nil
}

func createClusterPair(pairInfo map[string]string, skipStorage, resetConfig bool, clusterPairDir, projectIDMappings string) error {
	err := os.MkdirAll(path.Join(specDir, clusterPairDir), 0777)
	if err != nil {
		logrus.Errorf("Unable to make directory (%v) for cluster pair spec: %v", specDir+"/"+clusterPairDir, err)
		return err
	}
	clusterPairFileName := path.Join(specDir, clusterPairDir, pairFileName)
	pairFile, err := os.Create(clusterPairFileName)
	if err != nil {
		logrus.Errorf("Unable to create clusterPair.yaml: %v", err)
		return err
	}
	defer func() {
		err := pairFile.Close()
		if err != nil {
			logrus.Errorf("Error closing pair file: %v", err)
		}
	}()

	factory := storkctl.NewFactory()
	cmd := storkctl.NewCommand(factory, os.Stdin, pairFile, os.Stderr)

	if !skipStorage {
		var storageOptionsStr string
		if len(pairInfo) > 0 {
			for k, v := range pairInfo {
				if len(storageOptionsStr) > 0 {
					storageOptionsStr = storageOptionsStr + "," + k + "=" + v
				} else {
					storageOptionsStr = k + "=" + v
				}
			}
		}
		cmd.SetArgs([]string{"generate", "clusterpair", remotePairName, "--kubeconfig", remoteFilePath, "--project-mappings", projectIDMappings, "--storageoptions", storageOptionsStr})
	} else {
		cmd.SetArgs([]string{"generate", "clusterpair", remotePairName, "--kubeconfig", remoteFilePath, "--project-mappings", projectIDMappings})
	}
	if err := cmd.Execute(); err != nil {
		logrus.Errorf("Execute storkctl failed: %v", err)
		return err
	}

	if resetConfig {
		// storkctl generate command sets sched-ops to source cluster config
		err = setSourceKubeConfig()
		if err != nil {
			logrus.Errorf("during cluster pair setting kubeconfig to source failed %v", err)
			return err
		}
	} else {
		// Change kubeconfig to destination cluster config
		err = setDestinationKubeConfig()
		if err != nil {
			logrus.Errorf("during cluster pair setting kubeconfig to destination failed %v", err)
			return err
		}
	}

	logrus.Info("cluster-pair.yml created")
	return nil

}

func scheduleClusterPair(ctx *scheduler.Context, skipStorage, resetConfig bool, clusterPairDir, projectIDMappings string, reverse bool) error {
	var token string
	var err error
	if reverse {
		// For auth-enabled clusters first get admin token for destination cluster
		err := setSourceKubeConfig()
		if err != nil {
			return fmt.Errorf("during cluster pair setting kubeconfig to source failed %v", err)
		}
	} else {
		err := setDestinationKubeConfig()
		if err != nil {
			return fmt.Errorf("during cluster pair setting kubeconfig to destination failed %v", err)
		}
	}

	// For auth-enabled clusters, get token for the current cluster, which will be used to generate cluster pair
	if authTokenConfigMap != "" {
		token, err = getTokenFromSecret(adminTokenSecretName, defaultAdminNamespace)
		if err != nil {
			return err
		}
	}

	info, err := volumeDriver.GetClusterPairingInfo(remoteFilePath, token)
	if err != nil {
		logrus.Errorf("Error writing to clusterpair.yml: %v", err)
		return err
	}

	err = createClusterPair(info, skipStorage, resetConfig, clusterPairDir, projectIDMappings)
	if err != nil {
		logrus.Errorf("Error creating cluster Spec: %v", err)
		return err
	}

	err = schedulerDriver.RescanSpecs(specDir, volumeDriverName)
	if err != nil {
		logrus.Errorf("Unable to parse spec dir: %v", err)
		return err
	}

	err = schedulerDriver.AddTasks(ctx,
		scheduler.ScheduleOptions{AppKeys: []string{clusterPairDir}})
	if err != nil {
		logrus.Errorf("Failed to schedule Cluster Pair Specs: %v", err)
		return err
	}

	err = schedulerDriver.WaitForRunning(ctx, defaultWaitTimeout, defaultWaitInterval)
	if err != nil {
		logrus.Errorf("Error waiting to get cluster pair in ready state: %v", err)
		return err
	}

	return nil
}

// Create a cluster pair from source to destination and another cluster pair from destination to source
func scheduleBidirectionalClusterPair(cpName, cpNamespace, projectMappings string) error {
	var token string
	// Setting kubeconfig to source because we will create bidirectional cluster pair based on source as reference
	err := setSourceKubeConfig()
	if err != nil {
		return fmt.Errorf("during cluster pair setting kubeconfig to source failed %v", err)
	}

	// Create namespace for the cluster pair on source cluster
	_, err = core.Instance().CreateNamespace(&v1.Namespace{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: cpNamespace,
			Labels: map[string]string{
				"creator": "stork-test",
			},
		},
	})
	if err != nil {
		return fmt.Errorf("Failed to create namespace %s on source cluster", cpNamespace)
	}

	// Create directory to store kubeconfig files
	err = os.MkdirAll(path.Join(tempDir, bidirectionalClusterPairDir), 0777)
	if err != nil {
		logrus.Errorf("Unable to make directory (%v) for cluster pair spec: %v", tempDir+"/"+bidirectionalClusterPairDir, err)
		return err
	}
	srcKubeconfigPath := path.Join(tempDir, bidirectionalClusterPairDir, "src_kubeconfig")
	srcKubeConfig, err := os.Create(srcKubeconfigPath)
	if err != nil {
		logrus.Errorf("Unable to write source kubeconfig file: %v", err)
		return err
	}

	defer func() {
		err := srcKubeConfig.Close()
		if err != nil {
			logrus.Errorf("Error closing source kubeconfig file: %v", err)
		}
	}()

	// Dump source config to the directory created before
	err = dumpKubeConfigPath(srcConfig, srcKubeconfigPath)
	if err != nil {
		return fmt.Errorf("unable to dump remote config while setting source config: %v", err)
	}

	// For auth-enabled clusters, get token for the current cluster, which will be used to generate cluster pair
	if authTokenConfigMap != "" {
		token, err = getTokenFromSecret(adminTokenSecretName, defaultAdminNamespace)
		if err != nil {
			return err
		}
	}

	// Get cluster pair details for source cluster
	srcInfo, err := volumeDriver.GetClusterPairingInfo(srcKubeconfigPath, token)
	if err != nil {
		logrus.Errorf("Error writing to clusterpair.yml: %v", err)
		return err
	}

	destKubeconfigPath := path.Join(tempDir, bidirectionalClusterPairDir, "dest_kubeconfig")
	destKubeConfig, err := os.Create(destKubeconfigPath)
	if err != nil {
		logrus.Errorf("Unable to write source kubeconfig file: %v", err)
		return err
	}

	defer func() {
		err := destKubeConfig.Close()
		if err != nil {
			logrus.Errorf("Error closing destination kubeconfig file: %v", err)
		}
	}()

	// Dump destination config to the directory created before
	err = dumpKubeConfigPath(destConfig, destKubeconfigPath)
	if err != nil {
		return fmt.Errorf("unable to dump remote config while setting destination config: %v", err)
	}

	err = setDestinationKubeConfig()
	if err != nil {
		return fmt.Errorf("during cluster pair setting kubeconfig to source failed %v", err)
	}

	// For auth-enabled clusters, get token for the current cluster, which will be used to generate cluster pair
	if authTokenConfigMap != "" {
		token, err = getTokenFromSecret(adminTokenSecretName, defaultAdminNamespace)
		if err != nil {
			return err
		}
	}

	// Get cluster pair details for destination cluster
	destInfo, err := volumeDriver.GetClusterPairingInfo(destKubeconfigPath, token)
	if err != nil {
		logrus.Errorf("Error writing to clusterpair.yml: %v", err)
		return err
	}

	// Create namespace for the cluster pair on destination cluster
	_, err = core.Instance().CreateNamespace(&v1.Namespace{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: cpNamespace,
			Labels: map[string]string{
				"creator": "stork-test",
			},
		},
	})
	if err != nil {
		return fmt.Errorf("Failed to create namespace %s on destination cluster", cpNamespace)
	}

	err = setSourceKubeConfig()
	if err != nil {
		return fmt.Errorf("during cluster pair setting kubeconfig to source failed %v", err)
	}

	// Create source --> destination and destination --> cluster pairs using storkctl
	factory := storkctl.NewFactory()
	cmd := storkctl.NewCommand(factory, os.Stdin, os.Stdout, os.Stderr)
	cmd.SetArgs([]string{"create", "clusterpair", "-n", cpNamespace, cpName,
		"--src-kube-file", srcKubeconfigPath,
		"--src-ip", srcInfo[clusterIP],
		"--src-token", srcInfo[tokenKey],
		"--dest-kube-file", destKubeconfigPath,
		"--dest-ip", destInfo[clusterIP],
		"--dest-token", destInfo[tokenKey],
		"--project-mappings", projectMappings,
	})

	if err := cmd.Execute(); err != nil {
		return fmt.Errorf("Creation of bidirectional cluster pair using storkctl failed: %v", err)
	}
	return nil
}
func setMockTime(t *time.Time) error {
	timeString := ""
	if t != nil {
		timeString = t.Format(time.RFC1123)
	}

	cm, err := core.Instance().GetConfigMap(schedule.MockTimeConfigMapName, schedule.MockTimeConfigMapNamespace)
	if err != nil {
		if !errors.IsNotFound(err) {
			return err
		}

		// create new config map
		data := map[string]string{
			schedule.MockTimeConfigMapKey: timeString,
		}

		cm := &v1.ConfigMap{
			ObjectMeta: meta_v1.ObjectMeta{
				Name:      schedule.MockTimeConfigMapName,
				Namespace: schedule.MockTimeConfigMapNamespace,
			},
			Data: data,
		}
		_, err = core.Instance().CreateConfigMap(cm)
		return err
	}

	// update existing config map
	cmCopy := cm.DeepCopy()
	if cmCopy.Data == nil {
		cmCopy.Data = make(map[string]string)
	}

	cmCopy.Data[schedule.MockTimeConfigMapKey] = timeString
	_, err = core.Instance().UpdateConfigMap(cmCopy)
	if err != nil {
		return err
	}

	time.Sleep(configMapSyncWaitTime)
	return nil
}

func createApp(t *testing.T, testID string) *scheduler.Context {

	ctxs, err := schedulerDriver.Schedule(testID,
		scheduler.ScheduleOptions{AppKeys: []string{"mysql-1-pvc"}, Scheduler: schedulerName})
	require.NoError(t, err, "Error scheduling task")
	require.Equal(t, 1, len(ctxs), "Only one task should have started")

	err = schedulerDriver.WaitForRunning(ctxs[0], defaultWaitTimeout, defaultWaitInterval)
	require.NoError(t, err, "Error waiting for pod to get to running state")

	scheduledNodes, err := schedulerDriver.GetNodesForApp(ctxs[0])
	require.NoError(t, err, "Error getting node for app")
	require.Equal(t, 1, len(scheduledNodes), "App should be scheduled on one node")

	volumeNames := getVolumeNames(t, ctxs[0])
	require.Equal(t, 1, len(volumeNames), "Should only have one volume")

	verifyScheduledNode(t, scheduledNodes[0], volumeNames)
	return ctxs[0]
}

func addTimestampSuffix(path string) string {
	t := time.Now()
	timeStampSuffix := t.Format("20060102150405")
	return fmt.Sprintf("%s-%s-%s", bucketPrefix, path, timeStampSuffix)
}

func addSecurityAnnotation(spec interface{}) error {
	// Adds annotations required for auth enabled runs
	configMap, err := core.Instance().GetConfigMap(authTokenConfigMap, "default")
	if err != nil {
		logrus.Errorf("Error reading config map: %v", err)
		return err
	}
	logrus.Debugf("Config Map details: %v", configMap.Data)
	if _, ok := configMap.Data[secretNameKey]; !ok {
		return fmt.Errorf("failed to get secret name from config map")
	}
	if _, ok := configMap.Data[secretNamespaceKey]; !ok {
		return fmt.Errorf("failed to get secret namespace from config map")
	}
	if obj, ok := spec.(*storageapi.StorageClass); ok {
		if obj.Parameters == nil {
			obj.Parameters = make(map[string]string)
		}
		obj.Parameters[secretName] = configMap.Data[secretNameKey]
		obj.Parameters[secretNamespace] = configMap.Data[secretNamespaceKey]
	} else if obj, ok := spec.(*v1.PersistentVolumeClaim); ok {
		if obj.Annotations == nil {
			obj.Annotations = make(map[string]string)
		}
		obj.Annotations[secretName] = configMap.Data[secretNameKey]
		obj.Annotations[secretNamespace] = configMap.Data[secretNamespaceKey]
	} else if obj, ok := spec.(*snapv1.VolumeSnapshot); ok {
		if obj.Metadata.Annotations == nil {
			obj.Metadata.Annotations = make(map[string]string)
		}
		obj.Metadata.Annotations[secretName] = configMap.Data[secretNameKey]
		obj.Metadata.Annotations[secretNamespace] = configMap.Data[secretNamespaceKey]
	} else if obj, ok := spec.(*appsapi.StatefulSet); ok {
		var pvcList []v1.PersistentVolumeClaim
		for _, pvc := range obj.Spec.VolumeClaimTemplates {
			if pvc.Annotations == nil {
				pvc.Annotations = make(map[string]string)
			}
			pvc.Annotations[secretName] = configMap.Data[secretNameKey]
			pvc.Annotations[secretNamespace] = configMap.Data[secretNamespaceKey]
			pvcList = append(pvcList, pvc)
		}
		obj.Spec.VolumeClaimTemplates = pvcList
	} else if obj, ok := spec.(*storkv1.ApplicationBackup); ok {
		if obj.Annotations == nil {
			obj.Annotations = make(map[string]string)
		}
		obj.Annotations[secretName] = configMap.Data[secretNameKey]
		obj.Annotations[secretNamespace] = configMap.Data[secretNamespaceKey]
	} else if obj, ok := spec.(*storkv1.ApplicationClone); ok {
		if obj.Annotations == nil {
			obj.Annotations = make(map[string]string)
		}
		obj.Annotations[secretName] = configMap.Data[secretNameKey]
		obj.Annotations[secretNamespace] = configMap.Data[secretNamespaceKey]
	} else if obj, ok := spec.(*storkv1.ApplicationRestore); ok {
		if obj.Annotations == nil {
			obj.Annotations = make(map[string]string)
		}
		obj.Annotations[secretName] = configMap.Data[secretNameKey]
		obj.Annotations[secretNamespace] = configMap.Data[secretNamespaceKey]
	} else if obj, ok := spec.(*storkv1.Migration); ok {
		if obj.Annotations == nil {
			obj.Annotations = make(map[string]string)
		}
		obj.Annotations[secretName] = configMap.Data[secretNameKey]
		obj.Annotations[secretNamespace] = configMap.Data[secretNamespaceKey]
	} else if obj, ok := spec.(*storkv1.VolumeSnapshotRestore); ok {
		if obj.Annotations == nil {
			obj.Annotations = make(map[string]string)
		}
		obj.Annotations[secretName] = configMap.Data[secretNameKey]
		obj.Annotations[secretNamespace] = configMap.Data[secretNamespaceKey]
	} else if obj, ok := spec.(*storkv1.GroupVolumeSnapshot); ok {
		if obj.Annotations == nil {
			obj.Annotations = make(map[string]string)
		}
		obj.Annotations[secretName] = configMap.Data[secretNameKey]
		obj.Annotations[secretNamespace] = configMap.Data[secretNamespaceKey]
	} else if obj, ok := spec.(*storkv1.ApplicationBackupSchedule); ok {
		if obj.Annotations == nil {
			obj.Annotations = make(map[string]string)
		}
		obj.Annotations[secretName] = configMap.Data[secretNameKey]
		obj.Annotations[secretNamespace] = configMap.Data[secretNamespaceKey]
	} else if obj, ok := spec.(*storkv1.SchedulePolicy); ok {
		if obj.Annotations == nil {
			obj.Annotations = make(map[string]string)
		}
		obj.Annotations[secretName] = configMap.Data[secretNameKey]
		obj.Annotations[secretNamespace] = configMap.Data[secretNamespaceKey]
	} else if obj, ok := spec.(*storkv1.VolumeSnapshotSchedule); ok {
		if obj.Annotations == nil {
			obj.Annotations = make(map[string]string)
		}
		obj.Annotations[secretName] = configMap.Data[secretNameKey]
		obj.Annotations[secretNamespace] = configMap.Data[secretNamespaceKey]
	}
	return nil
}

func getStorkVersion(fullVersion string) string {
	noHash := strings.Split(fullVersion, "-")[0]
	majorVersion := strings.Split(noHash, ".")
	return strings.Join([]string{majorVersion[0], majorVersion[1]}, ".")
}

func getTokenFromSecret(secretName, secretNamespace string) (string, error) {
	var token string
	secret, err := core.Instance().GetSecret(secretName, secretNamespace)
	if err != nil {
		return "", fmt.Errorf("Failed to get secret %s: %v", secretName, err)
	}
	if tk, ok := secret.Data["auth-token"]; ok {
		token = string(tk)
		return token, nil
	}
	return "", fmt.Errorf("secret does not contain key 'auth-token'")
}

func TestMain(m *testing.M) {
	flag.IntVar(&snapshotScaleCount,
		"snapshot-scale-count",
		10,
		"Number of volumes to use for scale snapshot test")
	flag.IntVar(&migrationScaleCount,
		"migration-scale-count",
		10,
		"Number of migrations to use for migration test")
	flag.IntVar(&backupScaleCount,
		"backup-scale-count",
		10,
		"Number of different backups per app for scaled backup test")
	flag.StringVar(&volumeDriverName,
		"volume-driver",
		"pxd",
		"Stork volume driver to be used for stork integration tests")
	flag.StringVar(&genericCsiConfigMap,
		"generic-csi-config",
		"",
		"Config map name that contains details of csi driver to be used for provisioning")
	flag.BoolVar(&storkVersionCheck,
		"stork-version-check",
		false,
		"Turn on/off stork version check before running tests. Default off.")
	flag.Parse()
	if err := setup(); err != nil {
		logrus.Errorf("Setup failed with error: %v", err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}
