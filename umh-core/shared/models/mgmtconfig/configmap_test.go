package mgmtconfig_test

import (
	"context"
	"reflect"
	"time"

	"github.com/united-manufacturing-hub/ManagementConsole/shared/ptr"

	"github.com/united-manufacturing-hub/ManagementConsole/shared/helper"
	k8s_shared "github.com/united-manufacturing-hub/ManagementConsole/shared/kubernetes"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	uuidlib "github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/united-manufacturing-hub/ManagementConsole/shared/constants"
	mgmtconfig_helpers "github.com/united-manufacturing-hub/ManagementConsole/shared/mgmtconfig_helpers"
	"github.com/united-manufacturing-hub/ManagementConsole/shared/models/mgmtconfig"
)

var _ = Describe("Companion Configmap", func() {
	var (
		configMap corev1.ConfigMap
		cache     *k8s_shared.Cache
		clientset kubernetes.Interface
		data      map[string]string
	)

	BeforeEach(func() {
		By("setting the DEMO_MODE environment variable")
		// Set DEMO_MODE to true in order to use the fake kube clientset in this suite
		// instead of a real one
		helper.Setenv("DEMO_MODE", "true")

		var resourcecontroller *k8s_shared.ResourceController
		clientset, _, resourcecontroller, _ = k8s_shared.NewClientSet()
		cache = resourcecontroller.Cache

		By("deleting the default companion configmap")
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		Expect(clientset.CoreV1().ConfigMaps(constants.NamespaceMgmtCompanion).Delete(ctx, constants.ConfigMapName, metav1.DeleteOptions{})).To(Succeed())
		cancel()

		By("creating the companion configmap")
		data = make(map[string]string)

		data["umh"] = `enabled: true
lastUpdated: 0
version: 0.9.14
umh_merge_point: 3
helm_chart_addon: ""`

		data["connections"] = `- name: "Test_Connection"
  uuid: 6ba7b811-9dad-11d1-80b4-00c04fd430c8
  ip: "10.10.10.10"
  port: 4096
  type: "opcua-server"
  lastUpdated: 0
- name: "Test_Connection2"
  uuid: 75ad2485-11a3-4287-815a-626b33d27a35
  ip: "20.20.20.20"
  port: 4096
  type: "opcua-server"
  lastUpdated: 0
  notes: "Test Notes"`

		data["location"] = `enterprise: "UMH"
site: "Cologne"`
		data["datasource"] = `- name: "Test_Datasource"
  uuid: 26c922e5-64ce-4566-844f-2307c4aff66c
  connectionUUID: 6ba7b811-9dad-11d1-80b4-00c04fd430c8
  lastUpdated: 0
  nodes:
  - opcua_id: "ns=3;s=Test_Node"
    enterprise: "Test_Enterprise"
    site: "Test_Site"
    area: "Test_Area"
    line: "Test_Line"
    workcell: "Test_Workcell"
    tagname: "Test_Tagname"
    schema: "Test_Schema"
    benthos_addon: "Test_Benthos_Addon"
    tagType: "Test_TagType"
    originID: "Test_OriginID"
- name: "Test_Datasource2"
  uuid: 93aa027d-bb63-44f1-a460-87a432e13bd2
  connectionUUID: 75ad2485-11a3-4287-815a-626b33d27a35
  lastchange: 0
  subscribeEnabled: false
  nodes:
  - opcua_id: "ns=3;s=Test_Node2"
    enterprise: "Test_Enterprise2"
    site: "Test_Site2"
    area: "Test_Area2"
    line: "Test_Line2"
    workcell: "Test_Workcell2"
    tagname: "Test_Tagname2"
    schema: "Test_Schema2"
    benthos_addon: "Test_Benthos_Addon2"
    tagType: "Test_TagType2"
    originID: "Test_OriginID2"
  authentication:
    insecure: false
    username: "Test_Username"
    password: "Test_Password"
    certificatePub: "Test_CertificatePub"
    certificatePriv: "Test_CertificatePriv"`

		data["tls"] = `insecure-skip-tls-verify: false`

		data["brokers"] = `mqtt:
  ip: "192.168.123.456"
  port: 1883
  username: "test"
  password: "test"
  lastUpdated: 0`

		data["debug"] = `disableBackendConnection: true
updaterImageOverwrite: "1.0.0"`

		data["flags"] = `FF_UPDATER_INSECURE_MODE: true
FF_DEPLOYMENT_HEALTH_STATUS: true`

		configMap = corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      constants.ConfigMapName,
				Namespace: constants.NamespaceMgmtCompanion,
			},
			Data: data,
		}

		ctx, cancel = context.WithTimeout(context.Background(), 1*time.Second)
		Expect(clientset.CoreV1().ConfigMaps(configMap.Namespace).Create(ctx, &configMap, metav1.CreateOptions{})).ToNot(BeNil())
		cancel()
	})

	AfterEach(func() {
		By("deleting the companion configmap")
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		Expect(clientset.CoreV1().ConfigMaps(configMap.Namespace).Delete(ctx, configMap.Name, metav1.DeleteOptions{})).To(Succeed())
		cancel()
	})

	It("should get the companion configmap", func() {
		Eventually(func() bool {
			cfgMap, _ := mgmtconfig_helpers.GetMgmtConfig(cache)

			expected := corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.ConfigMapName,
					Namespace: constants.NamespaceMgmtCompanion,
				},
				Data: data,
			}
			expected.ResourceVersion = cfgMap.ResourceVersion

			return reflect.DeepEqual(cfgMap, expected)
		}, "30s").Should(BeTrue())

	})

	Context("extracting single fields from the configmap", func() {
		It("should extract the 'umh' field from the configmap", func() {
			umh, err := mgmtconfig_helpers.GetUMHFieldFromMgmtConfig(configMap)
			Expect(err).ToNot(HaveOccurred())
			Expect(umh).To(Equal(mgmtconfig.UmhConfig{
				Enabled:                    true,
				LastUpdated:                0,
				Version:                    "0.9.14",
				UmhMergePoint:              3,
				HelmChartAddon:             "",
				ReleaseChannel:             "stable",
				DisableHardwareStatusCheck: ptr.FalsePtr(),
			}))
		})

		It("should extract the 'connections' field from the configmap", func() {
			connections, err := mgmtconfig_helpers.GetConnectionsFieldFromMgmtConfig(configMap)
			Expect(err).ToNot(HaveOccurred())
			Expect(connections).To(Equal([]mgmtconfig.Connection{
				{
					Name:        "Test_Connection",
					Uuid:        uuidlib.MustParse("6ba7b811-9dad-11d1-80b4-00c04fd430c8"),
					Ip:          "10.10.10.10",
					Port:        4096,
					Type:        "opcua-server",
					LastUpdated: 0,
				},
				{
					Name:        "Test_Connection2",
					Uuid:        uuidlib.MustParse("75ad2485-11a3-4287-815a-626b33d27a35"),
					Ip:          "20.20.20.20",
					Port:        4096,
					Type:        "opcua-server",
					LastUpdated: 0,
					Notes:       "Test Notes",
				},
			}))
		})

		It("should extract the 'tls' field from the configmap", func() {
			tls, err := mgmtconfig_helpers.GetTlsFieldFromMgmtConfig(configMap)
			Expect(err).ToNot(HaveOccurred())
			Expect(tls).To(Equal(mgmtconfig.TlsConfig{
				InsecureSkipTLSVerify: false,
			}))
		})

		It("should extract the 'debug' field from the configmap", func() {
			debug, err := mgmtconfig_helpers.GetDebugFieldFromMgmtConfig(configMap)
			Expect(err).ToNot(HaveOccurred())
			Expect(debug).To(Equal(mgmtconfig.DebugConfig{
				DisableBackendConnection: true,
				UpdateTagOverwrite:       "1.0.0",
			}))
		})

		It("should extract the 'flags' field from the configmap", func() {
			flags := mgmtconfig_helpers.GetFlagsFieldFromMgmtConfig(configMap)
			Expect(flags).To(Equal(map[string]bool{
				"FF_UPDATER_INSECURE_MODE":    true,
				"FF_DEPLOYMENT_HEALTH_STATUS": true,
			}))
		})

		It("should extract the 'location' field from the configmap", func() {
			location, err := mgmtconfig_helpers.GetLocationFieldFromMgmtConfig(configMap)
			Expect(err).ToNot(HaveOccurred())
			Expect(location).To(Equal(mgmtconfig.Location{
				Enterprise: "UMH",
				Site:       "Cologne",
				Area:       "",
				Line:       "",
				WorkCell:   "",
			}))
		})

		// test setMQTTBroker set it and read it
		It("should extract the 'brokers' field from the configmap", func() {

			// _ := mgmtconfig.MQTTBroker{
			// 	Ip:          "192.168.123.456",
			// 	Port:        1883,
			// 	Username:    "test",
			// 	Password:    "test",
			// 	LastUpdated: 0,
			// }

			brokers := mgmtconfig.Brokers{
				MQTT: mgmtconfig.MQTTBroker{
					Ip:          "192.168.123.456",
					Port:        1883,
					Username:    "test",
					Password:    "test",
					LastUpdated: 0,
				},
			}

			returned_brokers, err := mgmtconfig_helpers.GetBrokersFieldFromMgmtConfig(configMap)
			Expect(err).ToNot(HaveOccurred())
			Expect(returned_brokers).To(Equal(brokers))

		})

	})
})
