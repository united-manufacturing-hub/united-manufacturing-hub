package runtime_config

import (
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/protocolconverterserviceconfig"
)

// BuildRuntimeConfig merges all variables (user + agent + global + internal),
// performs the location merge, derives the `bridged_by` header, and finally
// renders the three sub-templates.
// ─────────────
//
//   - *Spec* has been unmarshalled from YAML **and** already passed through the
//     variable-enrichment step performed by the control loop / manager.
//
//     👉  That means `spec.Variables` **already** contains
//     – user-supplied keys                 (flat)
//     – authoritative `.location` map      (merged from agent)
//     – fleet-wide  `.global`  namespace   (injected by central loop)
//     – runtime-only `.internal` namespace (added by the manager)
//
// Workflow
// ──────────────────────────────────────────────────────────────────────────────
// 1. Merge & normalize location maps:
//   - `agentLocation` – authoritative map from agent.location (may be nil)
//   - `pcLocation`    – optional overrides/extension from the PC spec
//   - Fill gaps with "unknown" up to highest defined level
//
// 2. Assemble complete variable bundle:
//   - Start with user bundle (flat)
//   - Add merged location map
//   - Add global variables if present
//   - Add internal namespace with PC ID
//   - Add bridged_by header derived from nodeName and pcName
//
// 3. Render all three sub-templates:
//   - Connection
//   - read-DFC   (with UNS **output** enforced)
//   - write-DFC  (with UNS **input**  enforced)
//
// Notes
// ─────
//   - The function is pure: it performs no side-effects and never mutates *Spec*.
//   - Passing a nil *Spec* results in an explicit error; an empty runtime
//     struct is **never** returned.
//   - After rendering, **no** `{{ … }}` directives remain.
//   - The returned object is ready for diffing or to be handed straight to the
//     Protocol-Converter FSM.
//
// It does NOT belong to the service
func BuildRuntimeConfig(
	spec protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec,
	agentLocation map[string]string,
	pcLocation map[string]string,
	globalVars map[string]any,
	nodeName string,
	pcName string,
) (protocolconverterserviceconfig.ProtocolConverterServiceConfigRuntime, error) {

	if reflect.DeepEqual(spec, protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec{}) {
		return protocolconverterserviceconfig.ProtocolConverterServiceConfigRuntime{},
			fmt.Errorf("nil spec")
	}

	//----------------------------------------------------------------------
	// 1. Merge & normalise *location* map
	//----------------------------------------------------------------------
	loc := map[string]string{}

	// 1a) copy agent levels (authoritative)
	for k, v := range agentLocation {
		loc[k] = v
	}

	// 1b) extend with PC-local additions (never overwrite agent keys)
	for k, v := range pcLocation {
		if _, exists := loc[k]; !exists {
			loc[k] = v
		}
	}

	// 1c) fill gaps up to the highest defined level with "unknown"
	maxLevel := -1
	for k := range loc {
		level, err := strconv.Atoi(k)
		if err == nil && level > maxLevel {
			maxLevel = level
		}
	}
	for i := 0; i <= maxLevel; i++ {
		key := strconv.Itoa(i)
		if _, exists := loc[key]; !exists {
			loc[key] = "unknown"
		}
	}

	//----------------------------------------------------------------------
	// 2. Assemble the **complete** variable bundle
	//----------------------------------------------------------------------
	vb := spec.Variables // start with user bundle (flat)
	if vb.User == nil {
		vb.User = map[string]any{}
	}
	vb.User["location"] = loc // merged map

	if len(globalVars) != 0 {
		vb.Global = globalVars
	}

	// Internal namespace
	vb.Internal = map[string]any{
		"id": pcName,
	}

	//----------------------------------------------------------------------
	// 3. bridged_by header
	//----------------------------------------------------------------------
	if nodeName == "" {
		nodeName = "unknown"
	}
	vb.Internal["bridged_by"] = generateProtocolConverterBridgedBy(nodeName, pcName)

	//----------------------------------------------------------------------
	// 4. Render all three sub-templates
	//----------------------------------------------------------------------
	scope := vb.Flatten()
	return renderConfig(spec, scope) // unexported helper that enforces UNS
}

// ---------------------------------------------------------------------
// Helper: derive a sanitised bridged_by value
// ---------------------------------------------------------------------
func generateProtocolConverterBridgedBy(nodeName, pcName string) string {
	bridgeName := fmt.Sprintf("protocol-converter-%s-%s", nodeName, pcName)

	reNonAlnum := regexp.MustCompile(`[^a-zA-Z0-9]`)
	bridgeName = reNonAlnum.ReplaceAllString(bridgeName, "-")

	reMultiDash := regexp.MustCompile(`-{2,}`)
	bridgeName = reMultiDash.ReplaceAllString(bridgeName, "-")

	return strings.Trim(bridgeName, "-")
}

// renderConfig turns the **author-facing** specification (*Spec*) into the
// **fully rendered** runtime configuration that the FSM compares against the
// live system.
//
// Preconditions
// ─────────────
//
//   - *Spec* has been unmarshalled from YAML **and** already passed through the
//     variable-enrichment step performed by the control loop / manager.
//
//     👉  That means `spec.Variables` **already** contains
//     – user-supplied keys                 (flat)
//     – authoritative `.location` map      (merged from agent)
//     – fleet-wide  `.global`  namespace   (injected by central loop)
//     – runtime-only `.internal` namespace (added by the manager)
//
//     renderConfig does **not** add or override any variables.
//     If a key is missing, template rendering will fail.
//
// Workflow
// ──────────────────────────────────────────────────────────────────────────────
// 1. Retrieve the three subordinate blueprints from *Spec*
//
//   - Connection
//
//   - read-DFC   (with UNS **output** enforced via GetDFCReadServiceConfig)
//
//   - write-DFC  (with UNS **input**  enforced via GetDFCWriteServiceConfig)
//
//     2. Render each blueprint with the already-enriched variable scope using
//     `config.RenderTemplate`. After this step **no** `{{ … }}` directives
//     remain.
//
//     3. Assemble the concrete pieces into a
//     `ProtocolConverterServiceConfigRuntime` value and return it.
//
// Notes
// ─────
//   - The function is pure: it performs no side-effects and never mutates *Spec*.
//   - Passing a nil *Spec* results in an explicit error; an empty runtime
//     struct is **never** returned.
//
// The returned object is ready for diffing or to be handed straight to the
// Protocol-Converter FSM.
func renderConfig(
	spec protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec,
	scope map[string]any,
) (
	protocolconverterserviceconfig.ProtocolConverterServiceConfigRuntime,
	error,
) {
	if reflect.DeepEqual(spec, protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec{}) {
		return protocolconverterserviceconfig.ProtocolConverterServiceConfigRuntime{}, fmt.Errorf("protocolConverter config is nil")
	}

	// ─── Render the three sub-templates ─────────────────────────────
	// Ensure to use GetDFCReadServiceConfig(), etc. to get the uns input/output enforced
	conn, err := config.RenderTemplate(spec.GetConnectionServiceConfig(), scope)
	if err != nil {
		return protocolconverterserviceconfig.ProtocolConverterServiceConfigRuntime{}, err
	}

	read, err := config.RenderTemplate(spec.GetDFCReadServiceConfig(), scope)
	if err != nil {
		return protocolconverterserviceconfig.ProtocolConverterServiceConfigRuntime{}, err
	}

	write, err := config.RenderTemplate(spec.GetDFCWriteServiceConfig(), scope)
	if err != nil {
		return protocolconverterserviceconfig.ProtocolConverterServiceConfigRuntime{}, err
	}

	return protocolconverterserviceconfig.ProtocolConverterServiceConfigRuntime{
		ConnectionServiceConfig:             conn,
		DataflowComponentReadServiceConfig:  read,
		DataflowComponentWriteServiceConfig: write,
	}, nil
}
