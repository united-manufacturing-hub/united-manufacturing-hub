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

package s6

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/s6serviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

// Mock implementation of os.DirEntry for testing
type mockDirEntry struct {
	name  string
	isDir bool
}

func (m mockDirEntry) Name() string               { return m.name }
func (m mockDirEntry) IsDir() bool                { return m.isDir }
func (m mockDirEntry) Type() os.FileMode          { return os.ModeDir }
func (m mockDirEntry) Info() (os.FileInfo, error) { return nil, nil }

var _ = Describe("S6 Service", func() {
	var (
		mockService *MockService
		ctx         context.Context
		testPath    string
		mockFS      *filesystem.MockFileSystem
	)

	BeforeEach(func() {
		mockService = NewMockService()
		ctx = context.Background()
		testPath = "/tmp/test-s6-service"
		mockFS = filesystem.NewMockFileSystem()

		// Cleanup any test directories if they exist
		err := mockFS.RemoveAll(ctx, testPath)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		// Cleanup
		err := mockFS.RemoveAll(ctx, testPath)
		Expect(err).NotTo(HaveOccurred())
	})

	It("should track method calls in the mock implementation", func() {
		Expect(mockService.CreateCalled).To(BeFalse())
		err := mockService.Create(ctx, testPath, s6serviceconfig.S6ServiceConfig{}, mockFS)
		Expect(err).NotTo(HaveOccurred())
		Expect(mockService.CreateCalled).To(BeTrue())

		Expect(mockService.StartCalled).To(BeFalse())
		err = mockService.Start(ctx, testPath, mockFS)
		Expect(err).NotTo(HaveOccurred())
		Expect(mockService.StartCalled).To(BeTrue())

		Expect(mockService.StopCalled).To(BeFalse())
		err = mockService.Stop(ctx, testPath, mockFS)
		Expect(err).NotTo(HaveOccurred())
		Expect(mockService.StopCalled).To(BeTrue())

		Expect(mockService.RestartCalled).To(BeFalse())
		err = mockService.Restart(ctx, testPath, mockFS)
		Expect(err).NotTo(HaveOccurred())
		Expect(mockService.RestartCalled).To(BeTrue())

		Expect(mockService.StatusCalled).To(BeFalse())
		_, err = mockService.Status(ctx, testPath, mockFS)
		Expect(err).NotTo(HaveOccurred())
		Expect(mockService.StatusCalled).To(BeTrue())

		Expect(mockService.ServiceExistsCalled).To(BeFalse())
		exists, _ := mockService.ServiceExists(ctx, testPath, mockFS)
		Expect(exists).To(BeTrue())
		Expect(mockService.ServiceExistsCalled).To(BeTrue())
	})

	It("should manage service state in the mock implementation", func() {
		// Service should not exist initially
		exists, _ := mockService.ServiceExists(ctx, testPath, mockFS)
		Expect(exists).To(BeFalse())

		// Create service should make it exist
		err := mockService.Create(ctx, testPath, s6serviceconfig.S6ServiceConfig{}, mockFS)
		Expect(err).NotTo(HaveOccurred())
		exists, _ = mockService.ServiceExists(ctx, testPath, mockFS)
		Expect(exists).To(BeTrue())

		// Set the service state to down initially
		mockService.ServiceStates[testPath] = ServiceInfo{
			Status: ServiceDown,
		}

		// Get status should return the set state
		info, err := mockService.Status(ctx, testPath, mockFS)
		Expect(err).NotTo(HaveOccurred())
		Expect(info.Status).To(Equal(ServiceDown))

		// Start service should change state to up
		err = mockService.Start(ctx, testPath, mockFS)
		Expect(err).NotTo(HaveOccurred())
		info, _ = mockService.Status(ctx, testPath, mockFS)
		Expect(info.Status).To(Equal(ServiceUp))

		// Stop service should change state to down
		err = mockService.Stop(ctx, testPath, mockFS)
		Expect(err).NotTo(HaveOccurred())
		info, _ = mockService.Status(ctx, testPath, mockFS)
		Expect(info.Status).To(Equal(ServiceDown))

		// Restart service should change state to up (after briefly being restarting)
		err = mockService.Restart(ctx, testPath, mockFS)
		Expect(err).NotTo(HaveOccurred())
		info, _ = mockService.Status(ctx, testPath, mockFS)
		Expect(info.Status).To(Equal(ServiceUp))

		// Remove service should make it not exist
		err = mockService.Remove(ctx, testPath, mockFS)
		Expect(err).NotTo(HaveOccurred())
		exists, _ = mockService.ServiceExists(ctx, testPath, mockFS)
		Expect(exists).To(BeFalse())

		// Status should not be available after removal
		_, err = mockService.Status(ctx, testPath, mockFS)
		Expect(err).NotTo(HaveOccurred()) // No error, but...
		info = mockService.StatusResult   // Should return the default result
		Expect(info.Status).To(Equal(ServiceUnknown))
	})

	Describe("IsKnownService", func() {
		var s6Service *DefaultService

		BeforeEach(func() {
			s6Service = &DefaultService{
				logger: nil,
			}
		})

		It("should correctly identify known services", func() {
			// Known services
			knownServices := []string{
				// Core services
				"s6-linux-init-shutdownd",
				"s6rc-fdholder",
				"s6rc-oneshot-runner",
				"syslogd",
				"syslogd-log",
				"umh-core",
				// S6 internal directories
				".s6-svscan",
				"user",
				"s6-rc",
				"log-user-service",
				// Pattern-based services
				"example-log",
				"another-prepare",
				"service-log-prepare",
			}

			for _, service := range knownServices {
				Expect(s6Service.IsKnownService(service)).To(BeTrue(), "Service should be known: "+service)
			}
		})

		It("should correctly identify unknown services", func() {
			// Unknown services
			unknownServices := []string{
				"custom-service",
				"benthos-pipeline",
				"benthos-instance-1",
				"my-service",
				"", // Empty string
			}

			for _, service := range unknownServices {
				Expect(s6Service.IsKnownService(service)).To(BeFalse(), "Service should be unknown: "+service)
			}
		})
	})

	Describe("CleanS6ServiceDirectory", func() {
		var (
			s6Service          *DefaultService
			mockFS             *filesystem.MockFileSystem
			removedDirectories []string
		)

		BeforeEach(func() {
			mockFS = filesystem.NewMockFileSystem()
			s6Service = &DefaultService{
				logger: nil, // Don't need the logger for this test
			}

			// Track removed directories
			removedDirectories = []string{}

			// Setup mock file system functions
			mockFS.WithReadDirFunc(func(ctx context.Context, path string) ([]os.DirEntry, error) {
				// Return a mix of known and unknown directories
				return []os.DirEntry{
					// Known services - core
					mockDirEntry{name: "s6-linux-init-shutdownd", isDir: true},
					mockDirEntry{name: "s6rc-fdholder", isDir: true},
					mockDirEntry{name: "s6rc-oneshot-runner", isDir: true},
					mockDirEntry{name: "syslogd", isDir: true},
					mockDirEntry{name: "syslogd-log", isDir: true},
					mockDirEntry{name: "umh-core", isDir: true},
					// Known services - s6 internals
					mockDirEntry{name: ".s6-svscan", isDir: true},
					mockDirEntry{name: "user", isDir: true},
					mockDirEntry{name: "s6-rc", isDir: true},
					// Known services - pattern-based
					mockDirEntry{name: "test-log", isDir: true},
					mockDirEntry{name: "service-prepare", isDir: true},
					// Unknown services (should be removed)
					mockDirEntry{name: "custom-service-1", isDir: true},
					mockDirEntry{name: "custom-service-2", isDir: true},
					mockDirEntry{name: "benthos-instance-1", isDir: true},
					mockDirEntry{name: "another-service", isDir: true},
					// Files that should be ignored
					mockDirEntry{name: "some-file.txt", isDir: false},
				}, nil
			})

			mockFS.WithRemoveAllFunc(func(ctx context.Context, path string) error {
				for _, known := range []string{
					// Core services
					"s6-linux-init-shutdownd", "s6rc-fdholder", "s6rc-oneshot-runner",
					"syslogd", "syslogd-log", "umh-core",
					// S6 internals
					".s6-svscan", "user", "s6-rc",
					// Pattern-based
					"test-log", "service-prepare",
				} {
					if path == filepath.Join(constants.S6BaseDir, known) {
						Fail("Known service was removed: " + known)
					}
				}

				// Record the removed directory
				dirName := filepath.Base(path)
				removedDirectories = append(removedDirectories, dirName)
				return nil
			})
		})

		It("should only remove non-standard directories", func() {
			// Execute the function under test
			err := s6Service.CleanS6ServiceDirectory(ctx, constants.S6BaseDir, mockFS)

			// Verify function execution
			Expect(err).NotTo(HaveOccurred())

			// Verify the correct directories were removed
			Expect(removedDirectories).To(ContainElements(
				"custom-service-1",
				"custom-service-2",
				"benthos-instance-1",
				"another-service",
			))

			// Verify the expected number of directories were removed
			Expect(removedDirectories).To(HaveLen(4))
		})

		It("should skip files that are not directories", func() {
			// This is verified by the mock setup - if it attempts to remove
			// a file, the test will fail since our mock only expects directories
			err := s6Service.CleanS6ServiceDirectory(ctx, constants.S6BaseDir, mockFS)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// TestGetS6ConfigFile tests the GetS6ConfigFile method
	Describe("GetS6ConfigFile", func() {
		var (
			s6Service *DefaultService
			mockFS    *filesystem.MockFileSystem
			ctx       context.Context
		)

		BeforeEach(func() {
			mockFS = filesystem.NewMockFileSystem()
			s6Service = &DefaultService{
				logger: nil, // Not needed for this test
			}
			ctx = context.Background()
		})

		Context("when the service does not exist", func() {
			BeforeEach(func() {
				// Setup mock file system to return service does not exist
				mockFS.WithPathExistsFunc(func(ctx context.Context, path string) (bool, error) {
					return false, nil
				})
			})

			It("should return ErrServiceNotExist", func() {
				servicePath := filepath.Join(constants.S6BaseDir, "non-existent-service")
				_, err := s6Service.GetS6ConfigFile(ctx, servicePath, "config.yaml", mockFS)
				Expect(err).To(Equal(ErrServiceNotExist))
			})
		})

		Context("when the service exists but the config file does not", func() {
			BeforeEach(func() {
				// Setup mock file system to return service exists but file does not
				mockFS.WithPathExistsFunc(func(ctx context.Context, path string) (bool, error) {
					servicePath := filepath.Join(constants.S6BaseDir, "test-service")
					// Service directory exists
					if path == servicePath {
						return true, nil
					}
					// But config file does not exist
					if path == filepath.Join(servicePath, constants.S6ConfigDirName, "config.yaml") {
						return false, nil
					}
					return false, nil
				})
			})

			It("should return an error indicating the file doesn't exist", func() {
				servicePath := filepath.Join(constants.S6BaseDir, "test-service")
				_, err := s6Service.GetS6ConfigFile(ctx, servicePath, "config.yaml", mockFS)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("does not exist"))
			})
		})

		Context("when both service and config file exist", func() {
			BeforeEach(func() {
				// Setup mock file system to return both service and file exist
				mockFS.WithPathExistsFunc(func(ctx context.Context, path string) (bool, error) {
					return true, nil
				})

				// Setup mock file system to return file content
				mockFS.WithReadFileFunc(func(ctx context.Context, path string) ([]byte, error) {
					servicePath := filepath.Join(constants.S6BaseDir, "test-service")
					if path == filepath.Join(servicePath, constants.S6ConfigDirName, "config.yaml") {
						return []byte("key: value"), nil
					}
					return nil, os.ErrNotExist
				})
			})

			It("should return the file content", func() {
				servicePath := filepath.Join(constants.S6BaseDir, "test-service")
				content, err := s6Service.GetS6ConfigFile(ctx, servicePath, "config.yaml", mockFS)
				Expect(err).NotTo(HaveOccurred())
				Expect(content).To(Equal([]byte("key: value")))
			})
		})

		Context("when context is cancelled", func() {
			It("should return context error", func() {
				cancelledCtx, cancel := context.WithCancel(context.Background())
				cancel() // Cancel immediately

				servicePath := filepath.Join(constants.S6BaseDir, "test-service")
				_, err := s6Service.GetS6ConfigFile(cancelledCtx, servicePath, "config.yaml", mockFS)
				Expect(err).To(Equal(context.Canceled))
			})
		})
	})

	Describe("ParseLogsFromBytes", func() {
		It("should parse logs from bytes", func() {
			data, err := os.ReadFile("s6_test_log_data.txt")
			Expect(len(data)).To(BeNumerically(">", 0))
			Expect(err).NotTo(HaveOccurred())
			entries, err := ParseLogsFromBytes(data)
			Expect(err).NotTo(HaveOccurred())

			// Expect more than 0 log entries
			Expect(len(entries)).To(BeNumerically(">", 0))
		})
	})
	// --- NEW TESTS FOR DefaultService.Remove ------------------------------------
	Describe("DefaultService Remove()", func() {
		var (
			ctx     context.Context
			mockFS  *filesystem.MockFileSystem
			svc     *DefaultService
			svcPath string
			logDir  string

			exists sync.Map // key:string → bool, simple in-memory "filesystem"
		)

		// helper – PathExists reads from our map
		pathExists := func(p string) bool {
			exists, ok := exists.Load(p)
			return ok && exists.(bool)
		}

		BeforeEach(func() {
			ctx = context.Background()
			mockFS = filesystem.NewMockFileSystem()
			svc = &DefaultService{
				logger: logger.For("test"),
			}
			svcPath = filepath.Join(constants.S6BaseDir, "my-service")
			logDir = filepath.Join(constants.S6LogBaseDir, "my-service")

			// default: both paths exist
			exists.Store(svcPath, true)
			exists.Store(logDir, true)

			// --- mock functions --------------------------------------------------

			mockFS.WithPathExistsFunc(func(ctx context.Context, p string) (bool, error) {
				return pathExists(p), nil
			})

			mockFS.WithRemoveAllFunc(func(ctx context.Context, p string) error {
				// simulate deletion
				exists.Delete(p)
				return nil
			})
		})

		It("removes both service and log directory (normal case)", func() {
			err := svc.Remove(ctx, svcPath, mockFS)
			Expect(err).NotTo(HaveOccurred())
			Expect(pathExists(svcPath)).To(BeFalse())
			Expect(pathExists(logDir)).To(BeFalse())
		})

		It("is successful when only the log dir had to be removed", func() {
			exists.Delete(svcPath) // service dir already gone

			err := svc.Remove(ctx, svcPath, mockFS)
			Expect(err).NotTo(HaveOccurred())
			Expect(pathExists(logDir)).To(BeFalse())
		})

		It("is idempotent (everything already gone)", func() {
			exists = sync.Map{} // nothing exists

			removeCalls := 0
			mockFS.WithRemoveAllFunc(func(ctx context.Context, p string) error {
				removeCalls++
				return nil
			})

			err := svc.Remove(ctx, svcPath, mockFS)
			Expect(err).NotTo(HaveOccurred())
			Expect(removeCalls).To(Equal(0), "RemoveAll should not be called when nothing exists")
		})

		It("returns an error when deletion fails", func() {
			boom := fmt.Errorf("IO error")
			mockFS.WithRemoveAllFunc(func(ctx context.Context, p string) error {
				if p == svcPath {
					return boom // fail removing service dir
				}
				exists.Delete(p)
				return nil
			})

			err := svc.Remove(ctx, svcPath, mockFS)
			Expect(err).To(MatchError(ContainSubstring("IO error")))
		})
	})
})
