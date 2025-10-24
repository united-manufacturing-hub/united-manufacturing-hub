// Copyright 2025 UMH Systems GmbH
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package config

import (
	"context"
	"fmt"
	"os"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/redpandaserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/env"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry"
	"go.uber.org/zap"
)

// LoadConfigWithEnvOverrides loads the config file and applies environment variable overrides.
// This function is used during initial application startup to handle configuration from both
// persistent config files and runtime environment variables passed via docker -e flags.
//
// Order of precedence (highest to lowest):
// 1. Environment variables (AUTH_TOKEN, API_URL, RELEASE_CHANNEL, ALLOW_INSECURE_TLS, LOCATION_*)
// 2. Existing config file values
// 3. Default values
//
// In Docker environments, this enables runtime configuration through environment variables,
// which is particularly useful for CI/CD pipelines, testing, and containerized deployments.
// For example, in the Makefile:
//
//	docker run -e AUTH_TOKEN=xyz -e LOCATION_0=factory1 -e ALLOW_INSECURE_TLS=true $(IMAGE_NAME):$(TAG)
//
// Detailed explanation of what happens:
//
// 1. Config file check:
//   - If the config file exists at /data/config.yaml, its contents are loaded
//   - If the file doesn't exist, a new config with default values is created
//
// 2. Environment variable processing:
//   - Environment variables are collected: AUTH_TOKEN, API_URL, RELEASE_CHANNEL, ALLOW_INSECURE_TLS, LOCATION_0..6
//   - Only non-empty variables will override existing config values
//   - For example, if AUTH_TOKEN is set in the environment, it will replace any existing value
//     in the config file
//
// 3. Config file persistence:
//   - The resulting configuration (with applied overrides) is written back to the config file
//   - This means environment variables cause PERMANENT changes to the config file
//   - On subsequent runs, these values become the baseline unless overridden again
//
// 4. Return value:
//   - The function returns the final configuration after all processing
//   - This config contains a mixture of:
//     a) Environment variable values (highest priority)
//     b) Existing config file values (if not overridden)
//     c) Default values (for any unspecified fields)
//
// Important: This function has side effects! It modifies the config file on disk.
func LoadConfigWithEnvOverrides(ctx context.Context, configManager *FileConfigManagerWithBackoff, log *zap.SugaredLogger) (FullConfig, error) {
	// Collect environment variables that can override config values
	authToken, err := env.GetAsString("AUTH_TOKEN", false, "")
	if err != nil {
		sentry.ReportIssuef(sentry.IssueTypeWarning, log, "Failed to get AUTH_TOKEN: %w", err)
	}

	apiURL, err := env.GetAsString("API_URL", false, "")
	if err != nil {
		sentry.ReportIssuef(sentry.IssueTypeWarning, log, "Failed to get API_URL: %w", err)
	}

	releaseChannel, err := env.GetAsString("RELEASE_CHANNEL", false, "")
	if err != nil {
		sentry.ReportIssuef(sentry.IssueTypeWarning, log, "Failed to get RELEASE_CHANNEL: %w", err)
	}

	// For AllowInsecureTLS, we need to know if it was explicitly set, so we first check the raw env var
	var (
		allowInsecureTLS    bool
		allowInsecureTLSSet bool
	)

	if allowInsecureTLSStr, exists := os.LookupEnv("ALLOW_INSECURE_TLS"); exists {
		allowInsecureTLS, err = env.GetAsBool("ALLOW_INSECURE_TLS", false, false)
		if err != nil {
			sentry.ReportIssuef(sentry.IssueTypeWarning, log, "Failed to parse ALLOW_INSECURE_TLS=%q as boolean: %w. Existing config value will be kept to avoid accidental security regressions", allowInsecureTLSStr, err)
		} else {
			allowInsecureTLSSet = true
		}
	}

	// Location values are numbered 0-6 and passed as LOCATION_0, LOCATION_1, etc.
	locations := make(map[int]string)

	for i := range 7 {
		location, err := env.GetAsString(fmt.Sprintf("LOCATION_%d", i), false, "")
		if err != nil {
			sentry.ReportIssuef(sentry.IssueTypeWarning, log, "Failed to get LOCATION_%d: %w", i, err)
		}

		locations[i] = location
	}

	// Build the config override structure from environment variables
	configOverride := FullConfig{
		Agent: AgentConfig{
			CommunicatorConfig: CommunicatorConfig{
				APIURL:    apiURL,
				AuthToken: authToken,
			},
			MetricsPort:    8080,
			ReleaseChannel: ReleaseChannel(releaseChannel),
			Location:       locations,
		},
		Internal: InternalConfig{
			Redpanda: RedpandaConfig{
				FSMInstanceConfig: FSMInstanceConfig{
					DesiredFSMState: "active", // Default desired state for Redpanda
				},
				RedpandaServiceConfig: redpandaserviceconfig.RedpandaServiceConfig{
					Topic: redpandaserviceconfig.TopicConfig{
						DefaultTopicRetentionMs:          constants.DefaultRedpandaTopicDefaultTopicRetentionMs,
						DefaultTopicRetentionBytes:       constants.DefaultRedpandaTopicDefaultTopicRetentionBytes,
						DefaultTopicCompressionAlgorithm: constants.DefaultRedpandaTopicDefaultTopicCompressionAlgorithm,
						DefaultTopicCleanupPolicy:        constants.DefaultRedpandaTopicDefaultTopicCleanupPolicy,
						DefaultTopicSegmentMs:            constants.DefaultRedpandaTopicDefaultTopicSegmentMs,
					},
					Resources: redpandaserviceconfig.ResourcesConfig{
						MaxCores: 1,
						// 2GB per core
						MemoryPerCoreInBytes: 2147483648,
					},
				},
			},
			TopicBrowser: TopicBrowserConfig{
				FSMInstanceConfig: FSMInstanceConfig{
					DesiredFSMState: "active",
				},
			},
		},
	}

	// Only set AllowInsecureTLS if it was explicitly provided as an environment variable
	if allowInsecureTLSSet {
		configOverride.Agent.AllowInsecureTLS = allowInsecureTLS
	}

	// Apply the environment overrides to the config
	configData, err := configManager.GetConfigWithOverwritesOrCreateNew(ctx, configOverride)
	if err != nil {
		return FullConfig{}, fmt.Errorf("failed to load config with environment overrides: %w", err)
	}

	return configData, nil
}
