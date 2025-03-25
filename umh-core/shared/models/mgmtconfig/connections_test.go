package mgmtconfig_test

import (
	"context"
	"time"

	"github.com/united-manufacturing-hub/ManagementConsole/shared/helper"
	k8s_shared "github.com/united-manufacturing-hub/ManagementConsole/shared/kubernetes"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"

	uuidlib "github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/united-manufacturing-hub/ManagementConsole/shared/constants"
	mgmtconfig_helpers "github.com/united-manufacturing-hub/ManagementConsole/shared/mgmtconfig_helpers"
	"github.com/united-manufacturing-hub/ManagementConsole/shared/models/mgmtconfig"
)

var _ = Describe("Connections", func() {
	var (
		clientset kubernetes.Interface
		cache     *k8s_shared.Cache
	)

	BeforeEach(func() {
		By("setting the DEMO_MODE environment variable")
		// Set DEMO_MODE to true in order to use the fake kube clientset in this suite
		// instead of a real one
		helper.Setenv("DEMO_MODE", "true")

		var resourcecontroller *k8s_shared.ResourceController
		clientset, _, resourcecontroller, _ = k8s_shared.NewClientSet()
		cache = resourcecontroller.Cache

		By("deleting the dafault companion configmap")
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		Expect(clientset.CoreV1().ConfigMaps(constants.NamespaceMgmtCompanion).Delete(ctx, constants.ConfigMapName, metav1.DeleteOptions{})).To(Succeed())
		cancel()

		By("creating the companion configmap")
		data := make(map[string]string)

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

		configMap := corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      constants.ConfigMapName,
				Namespace: constants.NamespaceMgmtCompanion,
			},
			Data: data,
		}

		ctx, cancel = context.WithTimeout(context.Background(), 1*time.Second)
		Expect(clientset.CoreV1().ConfigMaps(configMap.Namespace).Create(ctx, &configMap, metav1.CreateOptions{})).ToNot(BeNil())
		cancel()

		// wait for the configmap to be created
		time.Sleep(200 * time.Millisecond)
	})

	Describe("AddConnection", func() {
		It("should add a new connection", func() {
			newConnection := mgmtconfig.Connection{
				Name: "Test_Connection3",
				Uuid: uuidlib.New(),
				Ip:   "30.30.30.30",
				Port: 4096,
				Type: "opcua-server",
			}

			Expect(mgmtconfig_helpers.AddConnection(newConnection, cache, clientset)).To(Succeed())

			// wait for the configmap to be updated
			time.Sleep(200 * time.Millisecond)

			configMap, err := mgmtconfig_helpers.GetMgmtConfig(cache)
			Expect(err).ToNot(HaveOccurred())

			// here we use MatchFields to compare the elements of the connections slice
			// we ignore the LastUpdated field because it is set by the system
			Expect(mgmtconfig_helpers.GetConnectionsFieldFromMgmtConfig(configMap)).To(ContainElement(MatchFields(IgnoreExtras, Fields{
				"Name": Equal(newConnection.Name),
				"Ip":   Equal(newConnection.Ip),
				"Type": Equal(newConnection.Type),
				"Port": Equal(newConnection.Port),
				// Explicitly ignore LastUpdated by not mentioning it here
				"Uuid": Equal(newConnection.Uuid),
			})))
		})
		It("should fail to add a connection with the same UUID", func() {
			newConnection := mgmtconfig.Connection{
				Name: "Test_Connection",
				Uuid: uuidlib.MustParse("6ba7b811-9dad-11d1-80b4-00c04fd430c8"),
				Ip:   "10.10.10.10",
				Port: 4096,
				Type: "opcua-server",
			}

			Expect(mgmtconfig_helpers.AddConnection(newConnection, cache, clientset)).To(MatchError("connection with UUID 6ba7b811-9dad-11d1-80b4-00c04fd430c8 already exists"))
		})
	})

	AfterEach(func() {
		By("deleting the companion configmap")
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		Expect(clientset.CoreV1().ConfigMaps(constants.NamespaceMgmtCompanion).Delete(ctx, constants.ConfigMapName, metav1.DeleteOptions{})).To(Succeed())
		cancel()

		// wait for the configmap to be deleted
		time.Sleep(200 * time.Millisecond)
	})

	Describe("UpdateConnection", func() {
		It("should update an existing connection", func() {
			newConnection := mgmtconfig.Connection{
				Name: "Test_Connection_Updated",
				Uuid: uuidlib.MustParse("6ba7b811-9dad-11d1-80b4-00c04fd430c8"),
				Ip:   "10.10.10.10",
				Port: 4096,
				Type: "opcua-server",
			}

			Expect(mgmtconfig_helpers.UpdateConnection(newConnection, cache, clientset)).To(Succeed())

			// wait for the configmap to be updated
			time.Sleep(200 * time.Millisecond)

			configMap, err := mgmtconfig_helpers.GetMgmtConfig(cache)
			Expect(err).ToNot(HaveOccurred())

			// here we use MatchFields to compare the elements of the connections slice
			// we ignore the LastUpdated field because it is set by the system
			Expect(mgmtconfig_helpers.GetConnectionsFieldFromMgmtConfig(configMap)).To(ContainElement(MatchFields(IgnoreExtras, Fields{
				"Name": Equal(newConnection.Name),
				"Ip":   Equal(newConnection.Ip),
				"Type": Equal(newConnection.Type),
				"Port": Equal(newConnection.Port),
				"Uuid": Equal(newConnection.Uuid),
			})))
		})

		It("should fail to update a connection with a non-existing UUID", func() {
			uuid := uuidlib.New()
			newConnection := mgmtconfig.Connection{
				Name: "Test_Connection_Updated",
				Uuid: uuid,
				Ip:   "10.10.10.10",
				Port: 4096,
				Type: "opcua-server",
			}

			Expect(mgmtconfig_helpers.UpdateConnection(newConnection, cache, clientset)).To(MatchError("connection with UUID " + uuid.String() + " does not exist"))
		})
	})

	Describe("DeleteConnection", func() {
		It("should delete an existing connection", func() {
			connectionToDelete := mgmtconfig.Connection{
				Name: "Test_Connection",
				Uuid: uuidlib.MustParse("6ba7b811-9dad-11d1-80b4-00c04fd430c8"),
				Ip:   "10.10.10.10",
				Port: 4096,
				Type: "opcua-server",
			}

			Expect(mgmtconfig_helpers.DeleteConnection(uuidlib.MustParse("6ba7b811-9dad-11d1-80b4-00c04fd430c8"), cache, clientset)).To(Succeed())

			// wait for the configmap to be updated
			time.Sleep(200 * time.Millisecond)

			configMap, err := mgmtconfig_helpers.GetMgmtConfig(cache)
			Expect(err).ToNot(HaveOccurred())

			Expect(mgmtconfig_helpers.GetConnectionsFieldFromMgmtConfig(configMap)).ToNot(ContainElement(connectionToDelete))
		})

		It("should fail to delete a connection with a non-existing UUID", func() {
			uuid := uuidlib.New()
			Expect(mgmtconfig_helpers.DeleteConnection(uuid, cache, clientset)).To(MatchError("connection with UUID " + uuid.String() + " does not exist"))
		})
	})
})
