package constants

const MgmtMangedByLabel = "mgmtcompanion"
const NamespaceMgmtCompanion = "mgmtcompanion"
const NamespaceUMH = "united-manufacturing-hub"
const ConfigMapName = "mgmtcompanion-config"
const PodNameMgmtCompanion = "mgmtcompanion-0"

// BenthosUMHVersion IMPORTANT NOTE: When updating the following BenthosUMHVersion it is also important to update the benthos-umh
// go package within companion folder. The benthos-umh package inside the companion is used for OPC-Ua Node browser and
// benthos adhoc stream builder
const BenthosUMHVersion = "0.7.0"
const StatefulsetName = "mgmtcompanion"
const ConfigMapNameWAL = "mgmtcompanion-updater-wal"
const Secretname = "mgmtcompanion-secret"
const BackupCfgMapName = "mgmtcompanion-backup-metadata"
const BackupJobName = "mgmtupdater-job"
const BackupPVCName = "mgmtcompanion-backup"
const CompanionSecretName = "mgmtcompanion-secret"
const CompanionPVCName = "mgmtcompanion-pvc"
const UnsTimescaleDB = "timescaledb"
const UnsKafka = "kafka"
const UnsMqtt = "hivemqce"
const HelmReleaseName = "united-manufacturing-hub"
const HelmChartName = "united-manufacturing-hub/united-manufacturing-hub"

// ResolveReferenceURL already appends the index.yaml part
const HelmRepoIndex = "https://management.umh.app/helm/umh"

// HostPath is the path to where we map the hosts / directory to in the companion pod
// On Rocky Linux 9.X the / directory is on the same partition as the kubernetes volumes (usually sda2->rl-root on sata disks)
const HostPath = "/host"

// Constants regarding the Docker Images
const (
	DOCKER_REGISTRY        = "management.umh.app/oci"
	DOCKER_REPO            = "united-manufacturing-hub"
	DOCKER_IMAGE_COMPANION = "mgmtcompanion"
)

const (
	TOPIC_PREFIX = "umh.v1"
)

type SchemaName string

// Schema definitions
const (
	SCHEMA_ERROR_NO_SCHEMA_PRESENT SchemaName = "ERROR_NO_SCHEMA" // Do not add this to the ValidSchemas array
	SCHEMA_ERROR_NO_ENTERPRISE     SchemaName = "ERROR_NO_ENTERPRISE"
	SCHEMA_ANALYTICS               SchemaName = "_analytics"
	SCHEMA_HISTORIAN               SchemaName = "_historian"
	SCHEMA_LOCAL                   SchemaName = "_local"
)

// ValidSchemas is an array of strings that represents the valid schemas.
var ValidSchemas = [...]SchemaName{SCHEMA_HISTORIAN, SCHEMA_ANALYTICS, SCHEMA_LOCAL}
