package configurator

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/liferaft/kubekit/pkg/configurator/resources"

	"github.com/johandry/log"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"

	"github.com/kraken/ui"
	"github.com/liferaft/kubekit/pkg/configurator/ssh"
	"github.com/liferaft/kubekit/pkg/crypto"
	"github.com/liferaft/kubekit/pkg/manifest"
)

// TLSDirectory is the directory to upload the CA root certificates at the nodes
const TLSDirectory = "/etc/pki"

// Remote commands to check and install Ansible
const (
	CheckAnsibleCMD   = "test -f /usr/bin/ansible && echo OK || echo NOK"
	AnsibleVersionCMD = "/usr/bin/ansible --version 2>/dev/null | head -1"
	InstallAnsibleCMD = "test -f /usr/bin/ansible || ( sudo easy_install pip && sudo pip install ansible Flask )"
	GetEpochCMD       = "date +%s"
)

// KubekitBaseDir is the kubekit installation directory on every node
const KubekitBaseDir = "/var/kubekit"

// ConfiguratorBaseDir is the home directory to store the configurator files in
// every node
const ConfiguratorBaseDir = KubekitBaseDir + "/configurator"

// ConfiguratorLogDir is the log directory to store the configurator logs in
// every node
const ConfiguratorLogDir = "/var/log/kubekit"

//go:generate go run codegen/ansible/main.go --src ./templates/ansible --dst code.go --exclude *.bak

// variables containing the Ansible files such as the roles (in zip format),
// configuration and playbook. These variables values are generated by
// `codegen/ansible/main.go` using the go generate statement above.
var (
	Data       string
	AnsibleCfg string
	Callback   string
	Playbook   string
	Resources  map[string]string
)

// Configurator store all the settings from the cluster related to the Configurator
type Configurator struct {
	clusterName    string
	address        string
	port           int
	Hosts          Hosts
	stateData      map[string]interface{}
	platformConfig map[string]interface{}
	platform       string
	certPath       string
	inventory      *Inventory
	config         *Config
	resources      *resources.Resources
	ui             *ui.UI
}

// PodsPhaseCount tracks the count of the phases of the pods
type PodsPhaseCount struct {
	Total     uint32 // kubernetes doesn't even support as many pods as uint32 max
	Pending   uint32
	Running   uint32
	Succeeded uint32
	Failed    uint32
	Unknown   uint32
}

// AnsibleStatsMap is a type safe map wrapped with mutex locks around get/set calls
// where the keys are the role name and values are references to AnsibleStats
type AnsibleStatsMap struct {
	sync.RWMutex
	Results map[string]*AnsibleStats
}

// NewAnsibleStatsMap initializes/makes the AnsibleStatsMap map
func NewAnsibleStatsMap(size int) AnsibleStatsMap {
	return AnsibleStatsMap{
		Results: make(map[string]*AnsibleStats, size),
	}
}

// Store updates the value at the given key in the given AnsibleStatsMap
func (asm *AnsibleStatsMap) Store(key string, value *AnsibleStats) {
	asm.Lock()
	asm.Results[key] = value
	asm.Unlock()
}

// GetSnapshot returns a snapshot of the given AnsibleStatsMap
func (asm *AnsibleStatsMap) GetSnapshot() map[string]*AnsibleStats {
	asm.RLock()
	defer asm.RUnlock()
	return asm.Results
}

// New creates a configurator
func New(clusterName, platform, address string, port int, hosts Hosts, stateData map[string]interface{}, platformConfig interface{}, config *Config, res []string, basePath string, ui *ui.UI) (*Configurator, error) {
	pConfig := make(map[string]interface{})

	// DEBUG:
	// fmt.Printf("%s platform configuration: %+v\n", platform, platformConfig)

	pConfigB, err := json.Marshal(platformConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal the platform configuration. %s", err)
	}
	json.Unmarshal(pConfigB, &pConfig)

	var logPrefix string
	if len(platform) != 0 {
		logPrefix = fmt.Sprintf("@%s", platform)
	}
	if len(clusterName) != 0 {
		logPrefix = fmt.Sprintf("%s%s", clusterName, logPrefix)
	}
	if len(logPrefix) != 0 {
		logPrefix = fmt.Sprintf(" [ %s ]", logPrefix)
	}
	logPrefix = "Configurator" + logPrefix
	ui.SetLogPrefix(logPrefix)

	conf := Configurator{
		clusterName:    clusterName,
		address:        address,
		port:           port,
		Hosts:          hosts,
		stateData:      stateData,
		config:         config,
		platformConfig: pConfig,
		platform:       platform,
		ui:             ui,
		certPath:       filepath.Join(basePath, "certificates"),
	}

	kubeconfigPath := filepath.Join(conf.certPath, "kubeconfig")
	r, err := resources.New(kubeconfigPath, ui)
	if err != nil {
		return nil, err
	}
	r.AddResources(res)
	conf.resources = r

	if platform == "aks" {
		return &conf, nil
	}

	// DEBUG:
	// ui.Log.Debugf("resources loaded: %v", conf.resources.Names())

	username, ok := conf.platformConfig["username"]
	if !ok {
		return &conf, fmt.Errorf("not found username in %q platform configuration", platform)
	}
	privKey, err := GetPrivateKey(conf.platformConfig)
	if err != nil {
		return &conf, err
	}
	var password string
	passwordPtr, ok := conf.platformConfig["password"]
	if ok {
		password = passwordPtr.(string)
	}

	if len(privKey) == 0 && len(password) == 0 {
		return &conf, fmt.Errorf("not found password neither private key in %q platform configuration", platform)
	}

	// DEBUG:
	// parentLogger.Debugf("Password: %q\tKey: %q", password, privKey)

	if err := conf.Hosts.Config(username.(string), privKey, password, true); err != nil {
		return nil, err
	}

	return &conf, nil
}

// GetPrivateKey return the private key from the cluster platform configuration
func GetPrivateKey(platformConfig map[string]interface{}) (string, error) {
	if privateKey, ok := platformConfig["private_key"]; ok {
		return decryptKey(privateKey.(string))
	}

	privateKeyFile, ok := platformConfig["private_key_file"]
	if !ok || len(privateKeyFile.(string)) == 0 {
		return "", nil // fmt.Errorf("not found private key neither filename in platform configuration")
	}

	privateKey, err := ioutil.ReadFile(privateKeyFile.(string))
	if err != nil {
		return "", fmt.Errorf("failed to read private key from %s. %s", privateKeyFile, err)
	}

	return string(privateKey), nil
}

func decryptKey(key string) (string, error) {
	c, err := crypto.New(nil)
	if err != nil {
		return "", err
	}

	k, err := c.DecryptValue(key)
	return string(k), err
}

// Configure will start the process to configure the hosts to have Kubernetes up
// and running
func (c *Configurator) Configure() error {
	var errConfig error
	switch c.platform {
	case "eks", "aks":
	default:
		errConfig = c.configureWithAnsible()
	}

	if errResources := c.ApplyResources(false); errResources != nil {
		return errResources
	}

	// Ignore error, the validation may get the same error
	err := c.waitClusterReady()
	if err != nil {
		return err
	}

	errValidate := c.validateCluster()

	if errValidate != nil {
		if errConfig != nil {
			c.ui.Notify("kubernetes", fmt.Sprintf("%sconfiguration", ui.Red), "Kubernetes configuration and validation failed", "")
			c.ui.Log.Errorf("failed to validate the cluster. %s", errValidate)
			return fmt.Errorf("failed to configure and validate the cluster. Configuration error: %s. Validation error: %s", errConfig, errValidate)
		}
		c.ui.Notify("kubernetes", fmt.Sprintf("%svalidation", ui.Red), "Kubernetes validation failed", "")
		c.ui.Log.Warnf("the cluster configuration succeed without errors, but validation failed: %s", errValidate)
		return fmt.Errorf("failed to validate the cluster. %s", errValidate)
	}
	if errConfig != nil {
		c.ui.Log.Infof("the cluster validation succeed without errors")
		c.ui.Log.Warnf("the cluster configuration failed but the cluster looks healthy, report this error to the KubeKit team. %s", errConfig)
	}

	return nil
}

func (c *Configurator) addDataToResources() {
	switch c.platform {
	case "ec2":
		if v, ok := c.stateData["elastic-fileshares"]; ok {
			c.resources.AddData("elasticFileshares", v.(string))
		}
	case "eks":
		if v, ok := c.stateData["role-arn"]; ok {
			c.resources.AddData("roleARN", v.(string))
		} else {
			c.ui.Log.Warnf("variable %q was not found in the state", "role-arn")
		}
		if v, ok := c.stateData["elastic-fileshares"]; ok {
			c.resources.AddData("elasticFileshares", v.(string))
		}
		//if len(c.Hosts) > 0 {
		//	c.resources.AddData("heapsterNannyMemory", strconv.Itoa((len(c.Hosts)*200)+(90*1024))+"Ki")
		//} else {
		//	c.resources.AddData("heapsterNannyMemory", strconv.Itoa(200+(90*1024))+"Ki")
		//
		//}
		//c.resources.AddData("heapsterImageSrc", manifest.KubeManifest.Releases[manifest.Version].Dependencies.Core["heapster"].Src)
		//c.resources.AddData("addonResizerImageSrc", manifest.KubeManifest.Releases[manifest.Version].Dependencies.Core["addon-resizer"].Src)
		//c.resources.AddData("heapsterVersion", manifest.KubeManifest.Releases[manifest.Version].Dependencies.Core["heapster"].Version)
	case "aks":
		c.resources.AddData("EnableKubernetesDashboard", strconv.FormatBool(c.platformConfig["enable_kubernetes_dashboard"].(bool)))
		c.resources.AddData("AzureSubscriptionID", c.platformConfig["subscription_id"].(string))

		registry := c.platformConfig["container_registry"].(map[string]interface{})
		registryHost := registry["host"].(string)
		registryUser := registry["username"].(string)
		registryPass := registry["password"].(string)
		dockerConfigJSON := fmt.Sprintf(`{"auths":{"%s":{"username":"%s","password":"%s","auth":"%s"}}}`,
			registryHost,
			registryUser,
			registryPass,
			base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", registryUser, registryPass))),
		)
		dockerConfigJSONB64 := base64.StdEncoding.EncodeToString([]byte(dockerConfigJSON))
		c.resources.AddData("AzureACRDockerConfigJsonBase64", dockerConfigJSONB64)
	case "vsphere":
		if v, ok := c.stateData["server"]; ok {
			c.resources.AddData("vsphereServer", v.(string))
		} else {
			c.ui.Log.Warnf("variable %q was not found in the state", "server")
		}
		if v, ok := c.stateData["username"]; ok {
			c.resources.AddData("vsphereUsername", v.(string))
		} else {
			c.ui.Log.Warnf("variable %q was not found in the state", "username")
		}
		if v, ok := c.stateData["password"]; ok {
			c.resources.AddData("vspherePassword", v.(string))
		} else {
			c.ui.Log.Warnf("variable %q was not found in the state", "password")
		}
	}

	c.resources.AddData("kubekitVersion", manifest.Version)
	c.resources.AddData("clusterName", c.clusterName)
	c.resources.AddData("platform", c.platform)
	c.resources.AddData("certsPath", c.certPath)
}

// ApplyResources applies the Kubernetes manifests after rendering the templates.
// It may only export the rendered manifests if `export` is true.
func (c *Configurator) ApplyResources(export bool) error {
	c.addDataToResources()

	if export {
		return c.resources.Export(filepath.Join(c.certPath, "..", "kubernetes"))
	}

	return c.resources.ApplyAll()
}

func (c *Configurator) waitClusterReady() error {
	timer := time.Duration(10) * time.Minute
	logmsg := fmt.Sprintf("Default wait time set to %s", timer.String())
	tick := 10 * time.Second
	ticker := time.NewTicker(tick)
	defer ticker.Stop()

	if len(c.Hosts) > 10 && len(c.Hosts) <= 20 {
		timer = time.Duration(len(c.Hosts)) * time.Minute
		logmsg = fmt.Sprintf("Wait time set to %s based on number of nodes. Use `wait_for_ready: #` in the cluster config to manually set the wait time", timer.String())
	} else if len(c.Hosts) > 20 {
		timer = time.Duration(20) * time.Minute
		logmsg = fmt.Sprintf("Wait time set to %s based on number of nodes. Use `wait_for_ready: #` in the cluster config to manually set the wait time", timer.String())
	}

	if c.config != nil && c.config.WaitForReady > 0 {
		timer = time.Duration(c.config.WaitForReady) * time.Minute
		logmsg = fmt.Sprintf("Wait time set to %s from variable input `wait_for_ready`", timer.String())
	}

	c.ui.Log.Debugf("%s", logmsg)

	timeout := time.NewTicker(timer)
	defer timeout.Stop()

	kc := c.resources.KubernetesClient()

	var readyNodes, totalNodes int
	checkNodes := func() (err error) {
		if readyNodes, totalNodes, err = kc.NodesReady(); err != nil {
			return err
		}
		return nil
	}

	for {
		select {
		case <-ticker.C:
			// check the number of nodes up
			if err := checkNodes(); err != nil {
				return err
			}
			if readyNodes != 0 && totalNodes != 0 && readyNodes == totalNodes {
				c.ui.Log.Debugf("The cluster is ready with %d/%d nodes ready.", readyNodes, totalNodes)
				return nil
			}
			if totalNodes == 0 && readyNodes == totalNodes {
				c.ui.Log.Debugf("The cluster has %d/%d nodes defined.", readyNodes, totalNodes)
				return nil
			}
			c.ui.Log.Debugf("%d/%d nodes ready, waiting %s to try again", readyNodes, totalNodes, tick.String())
		case <-timeout.C:
			if err := checkNodes(); err != nil {
				return err
			}
			return fmt.Errorf("Wait for ready timeout hit. Not all nodes are ready after %s minutes (%d / %d)", timer.String(), readyNodes, totalNodes)
		}
	}
}

func (c *Configurator) getPodStatusInNS(namespace string) (*PodsPhaseCount, error) {
	pods, err := c.resources.KubernetesClient().ListPods(namespace)
	if err != nil {
		return nil, err
	}
	if len(pods.Items) == 0 {
		c.ui.Log.Infof("No pods found in namespace %s", namespace)
		return nil, nil
	}
	counts := PodsPhaseCount{
		Total: uint32(len(pods.Items)),
	}
	for _, p := range pods.Items {
		switch p.Status.Phase {
		case corev1.PodPending:
			counts.Pending++
		case corev1.PodRunning:
			counts.Running++
		case corev1.PodSucceeded:
			counts.Succeeded++
		case corev1.PodFailed:
			counts.Failed++
		case corev1.PodUnknown:
			counts.Unknown++
		}
		// Check at the bottom of this file, an example of the information available from a Pod
		c.ui.Log.Infof("Namespace: %s, Pod name: %q, Status: %s, On node: %s", namespace, p.Name, p.Status.Phase, p.Spec.NodeName)
	}

	return &counts, nil
}

func getWorseStatusColor(a string, b string) string {
	if b != ui.Green && b != ui.Yellow && b != ui.Red {
		panic(fmt.Sprint("Unsupported color type."))
	}

	switch a {
	case ui.Green:
		return b
	case ui.Yellow:
		if b == ui.Red {
			return b
		}
		return a
	case ui.Red:
		return a
	default:
		panic(fmt.Sprint("Unsupported color type."))
	}
}

// NOTE: assumes a namespace is not empty
func (c *Configurator) waitPodsReadyInNS(tries int, delay time.Duration, namespace string, currentStatusColor string) (string, error) {
	podReadyCount := uint32(0)
	var phaseCounts *PodsPhaseCount
	var err error

	for i := 0; i < tries; i++ {
		phaseCounts, err = c.getPodStatusInNS(namespace)
		if err != nil {
			return ui.Red, err
		}
		if phaseCounts != nil {
			podReadyCount = phaseCounts.Running + phaseCounts.Succeeded
			if podReadyCount == phaseCounts.Total {
				c.ui.Log.Debugf("the pods in the namespace %s are ready", namespace)
				break
			}
		}

		if i+1 < tries {
			c.ui.Log.Debugf("the pods in the namespace %s are not ready, waiting %s to try again", namespace, delay.String())
			time.Sleep(delay)
		}
	}

	statusColor := currentStatusColor
	color := ui.Green
	if podReadyCount == uint32(0) || phaseCounts == nil || phaseCounts.Total == 0 {
		color = ui.Red
		statusColor = ui.Red
	} else if podReadyCount < phaseCounts.Total {
		color = ui.Yellow
		statusColor = getWorseStatusColor(currentStatusColor, color)
	}

	task := fmt.Sprintf("%svalidation.pods.%s", color, namespace)
	podsMsg := fmt.Sprintf("Cluster %q in namespace %s has no pods Running/Succeeded",
		c.clusterName, namespace)
	if phaseCounts != nil {
		podsMsg = fmt.Sprintf("Cluster %q in namespace %s has %d pods Running/Succeeded out of a total of %d pods",
			c.clusterName, namespace, phaseCounts.Running+phaseCounts.Succeeded, phaseCounts.Total)
	}
	c.ui.Notify("kubernetes", task, podsMsg, "")
	c.ui.Log.Infof(podsMsg)

	return statusColor, nil
}

func (c *Configurator) validateCluster() error {
	nodes, err := c.resources.KubernetesClient().ListNodes()
	if err != nil {
		return err
	}
	if len(nodes.Items) == 0 {
		return fmt.Errorf("no nodes found in the cluster %q", c.clusterName)
	}
	readyCount := 0
	for _, n := range nodes.Items {
		// Check at the bottom of this file, an example of the information available from a Node
		status := "Not Ready"
		for _, c := range n.Status.Conditions {
			if c.Type == "Ready" && c.Status == "True" {
				status = "Ready"
				readyCount++
				break
			}
		}
		c.ui.Log.Infof("Node name: %q, Status: %s, Kubernetes version: %s, OS %s (kernel %s)",
			n.Name,
			status,
			n.Status.NodeInfo.KubeletVersion,
			n.Status.NodeInfo.OSImage,
			n.Status.NodeInfo.KernelVersion,
		)
	}

	statusColor := ui.Green

	color := ui.Green
	if readyCount == 0 {
		color = ui.Red
		statusColor = ui.Red
	} else if readyCount != len(nodes.Items) {
		color = ui.Yellow
		statusColor = ui.Yellow
	}
	task := fmt.Sprintf("%svalidation.nodes", color)
	nodesMsg := fmt.Sprintf("Cluster %q have %d/%d nodes Ready", c.clusterName, readyCount, len(nodes.Items))
	c.ui.Notify("kubernetes", task, nodesMsg, "")
	c.ui.Log.Infof(nodesMsg)

	statusColor, err = c.waitPodsReadyInNS(10, 30*time.Second, "kube-system", statusColor)
	if err != nil {
		return err
	}

	isRookInResources := false
	if c.resources != nil {
		for _, res := range c.resources.Names() {
			// we check for substring instead of doing a match to allow for a user to deploy their own rook-operator
			if strings.Contains(res, "rook-operator") {
				isRookInResources = true
			}
		}
	}

	if (c.inventory != nil && c.inventory.All.Variables.RookEnabled) || (c.resources != nil && isRookInResources) {
		statusColor, err = c.waitPodsReadyInNS(10, 60*time.Second, "rook-ceph-system", statusColor)
		if err != nil {
			return err
		}
		statusColor, err = c.waitPodsReadyInNS(10, 60*time.Second, "rook-ceph", statusColor)
		if err != nil {
			return err
		}
	}

	task = fmt.Sprintf("%sstatus", statusColor)
	statusMsg := fmt.Sprintf("The Kubernetes cluster %q is %sREADY%s", c.clusterName, statusColor, ui.Reset)

	switch color {
	case ui.Red:
		statusMsg = fmt.Sprintf("The Kubernetes cluster %q is %sNOT READY%s", c.clusterName, statusColor, ui.Reset)
	case ui.Yellow:
		statusMsg = fmt.Sprintf("The Kubernetes cluster %q was CREATED but %sNOT HEALTHY%s", c.clusterName, statusColor, ui.Reset)
	}
	c.ui.Log.Infof(statusMsg)
	c.ui.Notify("kubernetes", task, statusMsg, "")

	return nil
}

func (c *Configurator) configureWithAnsible() error {
	defer c.ui.TerminateAllNotifications("")

	if err := c.Setup(); err != nil {
		return err
	}
	c.UploadCerts()
	return c.RunPlaybook()
}

// UpdateHosts get the hosts from a Terraform state file
// TODO: Complete this function
func (c *Config) UpdateHosts(tfStateFile string) error {
	if _, err := os.Stat(tfStateFile); os.IsNotExist(err) {
		return fmt.Errorf("not found Terraform state file %s", tfStateFile)
	}

	return nil
}

func (c *Configurator) executeInHosts(hosts Hosts, wg *sync.WaitGroup, f func(host Host, logger *log.Logger)) {
	wg.Add(len(hosts))

	for _, host := range hosts {
		go f(host, c.ui.Log)
	}

	wg.Wait()
}

func (c *Configurator) executeInAllHosts(wg *sync.WaitGroup, f func(host Host, logger *log.Logger)) {
	c.executeInHosts(c.Hosts, wg, f)
}

// Setup installs and configures Ansible and upload the Ansible roles and inventory
func (c *Configurator) Setup() error {
	inventory, err := c.Inventory()
	if err != nil {
		return err
	}
	c.inventory = inventory

	inventoryYaml, err := inventory.Yaml()
	if err != nil {
		return fmt.Errorf("failed to create/marshal the inventory file: %s", err)
	}

	username := c.platformConfig["username"].(string)

	var wg sync.WaitGroup
	c.executeInAllHosts(&wg, func(host Host, logger *log.Logger) {
		defer wg.Done()
		defer host.ssh.Close()

		c.ui.Notify(host.RoleName, "setup", "<setup>", "", ui.Setup)
		defer c.ui.Notify(host.RoleName, "setup", "</setup>", "", ui.Setup)

		checkTime(host, logger)

		// This call should be deprecated once the KubeOS is packaged with Ansible
		configureAnsible(host, logger, username)

		uploadRoles(host, logger)

		uploadInventory(host, logger, string(inventoryYaml))
	})

	return nil
}

// UploadCerts uploads the certificate files located in the 'certificates/'
// directory in the cluster configuration dir
func checkTime(host Host, logger *log.Logger) {
	const offset = 180 // offset allowed in seconds

	//get local time before we run remote commands.
	localTime := time.Now()

	cmdChk := &ssh.Command{Command: GetEpochCMD}

	if err := host.ssh.Start(cmdChk); err != nil {
		logger.Errorf("[%s] failed to run the command %q: %s", host.RoleName, cmdChk, err)
		return
	}

	remoteSec, _ := strconv.ParseInt(strings.TrimRight(cmdChk.Stdout.String(), "\n"), 10, 64)
	remoteTime := time.Unix(remoteSec, 0)
	offsetTime := remoteTime.Add(time.Second * offset)
	difference := localTime.Sub(remoteTime)

	//Certificate times are done in GMT. As long as the hosts are with in `offset` seconds of the certificate creation host, all should be good
	logger.Debugf("[%s] Verifying that remote time is within %d seconds of localtime", host.RoleName, offset)

	if localTime.After(offsetTime) {
		logger.Errorf("[%s] Remote time(%d) is behind local time(%d) by %v which is greater than %d second allowed", host.RoleName, remoteTime.Unix(), localTime.Unix(), difference, offset)
		logger.Debugf("[%s] Difference in seconds between local and remote: %f . Certificates may be invalid", host.RoleName, difference.Seconds())
		return
	}
	if localTime.Before(remoteTime) {
		//as difference will be negative, invert for positive number
		difference *= -1
		logger.Debugf("[%s] Local time(%d) is before remote time(%d) by %v", host.RoleName, localTime.Unix(), remoteTime.Unix(), difference)
		logger.Debugf("[%s] Time difference is greather than %d seconds(%f) between local and remote", host.RoleName, offset, difference.Seconds())
		return
	}
	if localTime.After(remoteTime) {
		logger.Debugf("[%s] Local time(%d) is after remote time(%d) by %v. Allowed time difference is %d seconds", host.RoleName, localTime.Unix(), remoteTime.Unix(), difference, offset)
		logger.Debugf("[%s] Time differece is greather than %d seconds(%f) between local and remote", host.RoleName, offset, difference.Seconds())
		return
	}
}

// setupAnsible will install Ansible remotely into all the host.
// This function may be deprecated when the KubeOS image is packaged with Ansible
func configureAnsible(host Host, logger *log.Logger, name string) {
	handleErr := func(err error, cmd, hostname string) {
		if err != nil {
			logger.Errorf("[%s] failed to run the command %q: %s", hostname, cmd, err)
			return
		}
	}

	cmdChk := &ssh.Command{Command: CheckAnsibleCMD}
	cmdInstall := &ssh.Command{Command: InstallAnsibleCMD}
	cmdVer := &ssh.Command{Command: AnsibleVersionCMD}

	err := host.ssh.Start(cmdChk)
	handleErr(err, cmdChk.Command, host.RoleName)

	chkOutput := strings.TrimRight(cmdChk.Stdout.String(), "\n")
	if chkOutput != "OK" {
		logger.Infof("[%s] installing Ansible", host.RoleName)
		err := host.ssh.Start(cmdInstall)
		handleErr(err, cmdInstall.Command, host.RoleName)
	}

	err = host.ssh.Start(cmdVer)
	handleErr(err, cmdVer.Command, host.RoleName)
	verOutput := strings.TrimRight(cmdVer.Stdout.String(), "\n")
	verOutputFields := strings.Split(verOutput, " ")
	if verOutputFields[0] != "ansible" {
		logger.Errorf("[%s] failed to install ansible", host.RoleName)
		return
	}
	logger.Debugf("[%s] Ansible version %q", host.RoleName, verOutputFields[1])

	if err := host.ssh.SudoMkDir(ConfiguratorBaseDir); err != nil {
		logger.Errorf("[%s] failed to sudo create the configurator home directory %q: %s", host.RoleName, ConfiguratorBaseDir, err)
		return
	}
	logger.Infof("[%s] created configurator base directory on %s", host.RoleName, ConfiguratorBaseDir)

	if err := host.ssh.CreateGroup("kube"); err != nil {
		logger.Errorf("[%s] failed to create group 'kube'  %s", host.RoleName, err)
		return
	}
	logger.Infof("[%s] created group: kube", host.RoleName)

	if err := host.ssh.UserMod(name, "kube"); err != nil {
		logger.Errorf("[%s] failed to add user to 'kube' group %s", host.RoleName, err)
		return
	}
	logger.Infof("[%s] added user %s to 'kube' group", host.RoleName, name)

	if err := host.ssh.SetChown(KubekitBaseDir, name+":kube"); err != nil {
		logger.Errorf("[%s] failed to set ownership %s:kube on kubekit installation directory %q: %s", host.RoleName, name, KubekitBaseDir, err)
		return
	}
	logger.Debugf("[%s] kubekit installation directory ownership permssion '%s' set on %s", host.RoleName, name+":kube", KubekitBaseDir)

	if err := host.ssh.SetChown(ConfiguratorBaseDir, name+":kube"); err != nil {
		logger.Errorf("[%s] failed to set ownership %s:kube on configurator home directory %q: %s", host.RoleName, name, ConfiguratorBaseDir, err)
		return
	}
	logger.Debugf("[%s] configurator base directory ownership permssion '%s' set on %s", host.RoleName, name+":kube", ConfiguratorBaseDir)

	ansibleCfgFile := filepath.Join(ConfiguratorBaseDir, "ansible.cfg")

	if err := host.ssh.CreateFile(ansibleCfgFile, AnsibleCfg, 0644); err != nil {
		logger.Errorf("[%s] failed to create the ansible configuration file %q: %s", host.RoleName, ansibleCfgFile, err)
		return
	}
	logger.Debugf("[%s] created Ansible configuration file %s", host.RoleName, ansibleCfgFile)

	callbackFile := filepath.Join(ConfiguratorBaseDir, "kubekit.py")

	if err := host.ssh.CreateFile(callbackFile, Callback, 0644); err != nil {
		logger.Errorf("[%s] failed to create the ansible callback file %q: %s", host.RoleName, callbackFile, err)
		return
	}
	logger.Debugf("[%s] created Ansible callback file %s", host.RoleName, callbackFile)
}

// uploadRoles uploads the Ansible playbook, roles and other required files to
// all the nodes
func uploadRoles(host Host, logger *log.Logger) {
	// Backup the existing playbook, if any
	rolesPath := filepath.Join(ConfiguratorBaseDir, "roles")

	// bkpTime := time.Now().Format("2006-01-02T15:04:05")
	// cmdBkpRoles := &ssh.Command{Command: fmt.Sprintf("mv -f %s %s.%s.bkp 2>/dev/null || echo 'not found'", rolesPath, rolesPath, bkpTime)}
	cmdRmRoles := &ssh.Command{Command: fmt.Sprintf("test -d %s && rm -rf %s || echo 'not found'", rolesPath, rolesPath)}

	// Notify(host.RoleName, "setup", "Setting up configuration code...")

	err := host.ssh.Start(cmdRmRoles)
	if err != nil {
		logger.Errorf("[%s] failed to remove the roles: %s", host.RoleName, err)
		return
	}

	rmOutput := strings.TrimRight(cmdRmRoles.Stdout.String(), "\n")
	if rmOutput != "not found" {
		logger.Warnf("[%s] previous roles removed", host.RoleName)
	} else {
		logger.Debugf("[%s] no previous roles found", host.RoleName)
	}

	// Option #1: Create the Zip file and execute a command to unzip it.
	// It requires unzip command at the host:
	// -------------------------------------------------------------------------
	zipFile := filepath.Join(ConfiguratorBaseDir, "roles.zip")

	if err := host.ssh.CreateFile(zipFile, Data, 0644); err != nil {
		logger.Errorf("[%s] failed to create the roles zip file %q: %s", host.RoleName, zipFile, err)
		return
	}

	logger.Infof("[%s] zip file %s uploaded", host.RoleName, zipFile)

	cmdUnZip := &ssh.Command{Command: fmt.Sprintf("cd %s && unzip %s", ConfiguratorBaseDir, zipFile)}

	err = host.ssh.Start(cmdUnZip)
	if err != nil {
		logger.Errorf("[%s] failed to run the command %q: %s", host.RoleName, cmdUnZip.Command, err)
		return
	}
	logger.Debugf("[%s] zip file %s unzipped to %s", host.RoleName, zipFile, ConfiguratorBaseDir)

	// -------------------------------------------------------------------------
	// Option #2: (Best) Use host.ssh.UnZip to unzip the data directly to the
	// /tmp/ dir. Does not require unzip command:
	// -------------------------------------------------------------------------
	// if err := host.ssh.UnZip(rolesPath, Data); err != nil {
	// 	logger.Errorf("[%s] failed to create the roles on %s: %s", host.RoleName, rolesPath, err)
	// 	return
	// }
	// -------------------------------------------------------------------------

	// Upload the Playbook
	playbookFile := filepath.Join(ConfiguratorBaseDir, "kubekit.yml")

	hostPlaybook := strings.Replace(Playbook, "kube_cluster", host.RoleName, -1)

	if err := host.ssh.CreateFile(playbookFile, hostPlaybook, 0644); err != nil {
		logger.Errorf("[%s] failed to create the playbook file %q: %s", host.RoleName, playbookFile, err)
		return
	}
	logger.Debugf("[%s] created playbook file %s", host.RoleName, playbookFile)

	// Upload the MANIFEST
	manifestFile := filepath.Join(ConfiguratorBaseDir, "MANIFEST")

	manifestYaml, err := manifest.Yaml()
	if err != nil {
		logger.Errorf("failed to get the manifest in YAML format. %s", err)
		return
	}

	if err := host.ssh.CreateFile(manifestFile, string(manifestYaml), 0644); err != nil {
		logger.Errorf("[%s] failed to create the file %q: %s", host.RoleName, manifestFile, err)
		return
	}
	logger.Debugf("[%s] created file %s", host.RoleName, manifestFile)

	// Upload the VERSION
	versionFile := filepath.Join(ConfiguratorBaseDir, "VERSION")

	if err := host.ssh.CreateFile(versionFile, manifest.Version, 0644); err != nil {
		logger.Errorf("[%s] failed to create the file %q: %s", host.RoleName, versionFile, err)
		return
	}
	logger.Debugf("[%s] created file %s", host.RoleName, versionFile)
}

// UploadInventory uploads the inventory file to every node
func uploadInventory(host Host, logger *log.Logger, inventoryYaml string) {
	inventoryFile := filepath.Join(ConfiguratorBaseDir, "inventory.yml")

	if err := host.ssh.CreateFile(inventoryFile, inventoryYaml, 0644); err != nil {
		logger.Errorf("[%s] failed to create the inventory file %q: %s", host.RoleName, inventoryFile, err)
		return
	}
	logger.Infof("[%s] created inventory file %s", host.RoleName, inventoryFile)
}

// UploadCerts uploads the certificate files located in the 'certificates/'
// directory in the cluster configuration dir
func (c *Configurator) UploadCerts() {
	var wg sync.WaitGroup
	certsDir := filepath.Join(c.certPath, c.platform)
	c.executeInAllHosts(&wg, func(host Host, logger *log.Logger) {
		defer wg.Done()
		defer host.ssh.Close()

		targetDir := filepath.Join(ConfiguratorBaseDir, "certificates")

		if err := host.ssh.MkDir(targetDir); err != nil {
			logger.Errorf("[%s] failed to create the certificates directory %q: %s", host.RoleName, targetDir, err)
			return
		}
		c.ui.Notify(host.RoleName, "certificates", "<certificates>", "", ui.Upload)
		defer c.ui.Notify(host.RoleName, "certificates", "</certificates>", "")

		logger.Debugf("[%s] created certificates directory on %s", host.RoleName, targetDir)

		certFiles := []string{
			"etcd_node.crt",
			"etcd_node.key",
			"etcd_root_ca.crt",
			"node.crt",
			"node.key",
			"admin.crt",
			"admin.key",
			"kube_controller.crt",
			"kube_controller.key",
			"kube_proxy.crt",
			"kube_proxy.key",
			"kube_scheduler.crt",
			"kube_scheduler.key",
			"root_ca.crt",
			"srv_acc.key",
			"ingress.key",
			"ingress.crt",
		}

		certFilesPerHost := []string{
			"kubelet.crt",
			"kubelet.key",
		}

		for _, fileName := range certFiles {
			filePath := filepath.Join(certsDir, fileName)

			targetFilePath := filepath.Join(targetDir, fileName)

			content, err := ioutil.ReadFile(filePath)
			if err != nil {
				logger.Errorf("failed to read the certificate file %q: %s", filePath, err)
				return
			}
			if err := host.ssh.CreateFile(targetFilePath, string(content), 0644); err != nil {
				logger.Errorf("[%s] failed to upload the certificate file %q: %s", host.RoleName, targetFilePath, err)
				return
			}
			logger.Debugf("[%s] uploaded certificate file %s", host.RoleName, fileName)
		}

		// If there is a directory in the certs dir with the hostname, upload all the per host certs
		pubHostname := strings.Split(host.PublicDNS, ".")[0]
		certsDirPerHost := filepath.Join(certsDir, pubHostname)
		if _, err := os.Stat(certsDirPerHost); err == nil {
			for _, fileName := range certFilesPerHost {
				filePath := filepath.Join(certsDirPerHost, fileName)

				targetFilePath := filepath.Join(targetDir, fileName)

				content, err := ioutil.ReadFile(filePath)
				if err != nil {
					logger.Errorf("failed to read the certificate file %q: %s", filePath, err)
					return
				}
				if err := host.ssh.CreateFile(targetFilePath, string(content), 0644); err != nil {
					logger.Errorf("[%s] failed to upload the certificate file %q: %s", host.RoleName, targetFilePath, err)
					return
				}
				logger.Debugf("[%s] uploaded the %s certificate file %s", host.RoleName, pubHostname, fileName)
			}
		}

		cmdMvCerts := &ssh.Command{Command: fmt.Sprintf("sudo mv %s/*.{crt,key} %s/ && sudo chown root:kube %s/*.{crt,key} && sudo chmod 0640  %s/*.{crt,key} && echo OK", targetDir, TLSDirectory, TLSDirectory, TLSDirectory)}

		err := host.ssh.Start(cmdMvCerts)
		if err != nil {
			logger.Errorf("[%s] failed to move the certificates to %s: %s", host.RoleName, TLSDirectory, err)
			return
		}
		mvOutput := strings.TrimRight(cmdMvCerts.Stdout.String(), "\n")
		if mvOutput == "OK" {
			logger.Debugf("[%s] certificates moved to %s", host.RoleName, TLSDirectory)
		} else {
			logger.Errorf("[%s] failed to move the certificates to %s", host.RoleName, TLSDirectory)
		}

		TLSTrustDirectory := filepath.Join(TLSDirectory, "trust", "anchors")
		cmdMvRoot2Trust := &ssh.Command{Command: fmt.Sprintf("sudo cp %s/*_ca.crt %s/ && sudo /usr/sbin/update-ca-certificates && echo OK", TLSDirectory, TLSTrustDirectory)}

		err = host.ssh.Start(cmdMvRoot2Trust)
		if err != nil {
			logger.Errorf("[%s] failed to move the certificates to %s: %s", host.RoleName, TLSDirectory, err)
			return
		}
		mvOutput = strings.TrimRight(cmdMvRoot2Trust.Stdout.String(), "\n")
		if mvOutput == "OK" {
			logger.Debugf("[%s] root certificates copied to %s", host.RoleName, TLSTrustDirectory)
		} else {
			logger.Errorf("[%s] failed to copy the root certificates to %s", host.RoleName, TLSDirectory)
		}
	})

	masters := c.Hosts.FilterByRole("master")

	c.executeInHosts(masters, &wg, func(host Host, logger *log.Logger) {
		defer wg.Done()
		defer host.ssh.Close()

		targetDir := filepath.Join(ConfiguratorBaseDir, "certificates")
		// directory should exists, as it was created in previous remote execution

		filePath := filepath.Join(certsDir, "kubeconfig")
		targetFilePath := filepath.Join(targetDir, "remote-kubeconfig")

		content, err := ioutil.ReadFile(filePath)
		if err != nil {
			logger.Errorf("failed to read the kubeconfig file %q: %s", filePath, err)
			return
		}
		if err := host.ssh.CreateFile(targetFilePath, string(content), 0644); err != nil {
			logger.Errorf("[%s] failed to upload the kubeconfig file %q: %s", host.RoleName, targetFilePath, err)
			return
		}
		logger.Infof("[%s] uploaded kubeconfig file", host.RoleName)

		cmdMvKConf := &ssh.Command{Command: fmt.Sprintf("sudo mv %s/remote-kubeconfig /var/lib/kubelet/ && sudo chmod 0400  /var/lib/kubelet/remote-kubeconfig && echo OK", targetDir)}

		err = host.ssh.Start(cmdMvKConf)
		if err != nil {
			logger.Errorf("[%s] failed to move the kubeconfig to /var/lib/kubelet/remote-kubeconfig: %s", host.RoleName, err)
			return
		}
		mvOutput := strings.TrimRight(cmdMvKConf.Stdout.String(), "\n")
		if mvOutput == "OK" {
			logger.Debugf("[%s] kubeconfig moved to /var/lib/kubelet/remote-kubeconfig", host.RoleName)
			c.ui.Notify(host.RoleName, "certificates", "remote kubeconfig generated and uploaded", "")
		} else {
			logger.Errorf("[%s] failed to move the kubeconfig to /var/lib/kubelet/remote-kubeconfig", host.RoleName)
		}
	})
}

// RunPlaybook will execute ansible remotely to configure the host to have
// Kubernetes up and running.
func (c *Configurator) RunPlaybook() error {
	var wg sync.WaitGroup
	globalStats := NewAnsibleStatsMap(len(c.Hosts))

	name := c.platformConfig["username"].(string)

	c.executeInAllHosts(&wg, func(host Host, logger *log.Logger) {
		defer wg.Done()
		defer host.ssh.Close()

		if err := host.ssh.SudoMkDir(ConfiguratorLogDir); err != nil {
			logger.Errorf("[%s] failed to create the logs directory %q: %s", host.RoleName, ConfiguratorLogDir, err)
			return
		}

		if err := host.ssh.SetChown(ConfiguratorLogDir, name); err != nil {
			logger.Errorf("[%s] failed to set ownership %s on configurator log directory %q: %s", host.RoleName, name, ConfiguratorLogDir, err)
			return
		}
		logger.Debugf("[%s] configurator log directory ownership permssion set on %s", host.RoleName, ConfiguratorBaseDir)

		touchLogFile := &ssh.Command{Command: fmt.Sprintf("echo -ne '----------------\n\tNEW EXECUTION ( '$(date)' ) \n----------------\n' | sudo tee --append %s/configurator.log >/dev/null && echo OK", ConfiguratorLogDir)}
		err := host.ssh.Start(touchLogFile)
		if err != nil {
			logger.Errorf("[%s] failed to clean configurator logs: %s", host.RoleName, err)
			return
		}
		touchOutput := strings.TrimRight(touchLogFile.Stdout.String(), "\n")
		if touchOutput == "OK" {
			logger.Debugf("[%s] configurator log files cleaned", host.RoleName)
		} else {
			logger.Errorf("[%s] failed to clean configurator logs", host.RoleName)
		}

		// Notify(host.RoleName, "configuration", "Configuring...")

		doneAnsibleCh := make(chan bool, 1)
		ansible, err := newAnsibleClient(host, c.ui, &doneAnsibleCh)
		if err != nil {
			logger.Errorf("[%s] failed to create the Ansible client. %s", host.RoleName, err)
			return
		}
		go ansible.PrintLogs()

		ansibleDebug := ""
		if logger.Level == logrus.DebugLevel {
			ansibleDebug = "-vvv"
		}

		// Execute the playbook and make sure it's not running when finish
		ansibleCMD := `cd %s && \
			ansible-playbook %s -i inventory.yml -l %s kubekit.yml | sudo tee --append %s/configurator.log >/dev/null; \
			pid=$(ps -fea | grep [a]nsible | grep -v bash | awk '{print $2}'); \
			test -z ${pid} || kill -9 ${pid}`

		executePlaybook := &ssh.Command{Command: fmt.Sprintf(ansibleCMD, ConfiguratorBaseDir, ansibleDebug, host.RoleName, ConfiguratorLogDir)}

		err = host.ssh.Start(executePlaybook)
		doneAnsibleCh <- true
		if err != nil {
			logger.Errorf("[%s] failed to run the Ansible playbook: %s", host.RoleName, err)
			return
		}
		globalStats.Store(host.RoleName, ansible.Stats)

		configMsg := "Configuration complete on %s"
		color := ui.Green
		if ansible.Stats == nil {
			color = ui.Yellow
			configMsg = "Configuration complete on %s without stats"
		} else if !ansible.Stats.Ok() {
			color = ui.Red
			configMsg = "Configuration fail on %s"
		}
		configMsg = fmt.Sprintf(configMsg, host.RoleName)
		if ansible.Stats != nil && !ansible.Stats.Empty() {
			configMsg = fmt.Sprintf("%s after %gs", configMsg, ansible.Stats.Duration)
		}

		task := fmt.Sprintf("%sconfiguration", color)

		c.ui.Notify(host.RoleName, task, configMsg, "")
	})

	var ok bool
	for role, stat := range globalStats.GetSnapshot() {
		if stat != nil && stat.Ok() {
			c.ui.Log.Infof("[%s] is ready for Kubernetes. Duration: %gs", role, stat.Duration)
			ok = true
			continue
		}

		if stat == nil {
			c.ui.Log.Warnf("[%s] stats are missing, may failed to configure Kubernetes", role)
			continue
		}

		if !stat.Ok() {
			c.ui.Log.Errorf("[%s] failed to configure Kubernetes. Duration: %gs", role, stat.Duration)
		}
	}

	if !ok {
		return fmt.Errorf("failed to configure Kubernetes on the cluster")
	}
	return nil
}
