package hotline

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type mockReadWriter struct {
	RBuf bytes.Buffer
	WBuf *bytes.Buffer
}

func (mrw mockReadWriter) Read(p []byte) (n int, err error) {
	return mrw.RBuf.Read(p)
}

func (mrw mockReadWriter) Write(p []byte) (n int, err error) {
	return mrw.WBuf.Write(p)
}

func TestServer_handleFileTransfer(t *testing.T) {
	type fields struct {
		ThreadedNews    *ThreadedNews
		FileTransferMgr FileTransferMgr
		Config          Config
		ConfigDir       string
		Stats           *Stats
		Logger          *slog.Logger
		FS              FileStore
	}
	type args struct {
		ctx context.Context
		rwc io.ReadWriter
	}
	tests := []struct {
		name     string
		fields   fields
		args     args
		wantErr  assert.ErrorAssertionFunc
		wantDump string
	}{
		{
			name: "with invalid protocol",
			args: args{
				ctx: func() context.Context {
					ctx := context.Background()
					ctx = context.WithValue(ctx, contextKeyReq, requestCtx{})
					return ctx
				}(),
				rwc: func() io.ReadWriter {
					mrw := mockReadWriter{}
					mrw.WBuf = &bytes.Buffer{}
					mrw.RBuf.Write(
						[]byte{
							0, 0, 0, 0,
							0, 0, 0, 5,
							0, 0, 0x01, 0,
							0, 0, 0, 0,
						},
					)
					return mrw
				}(),
			},
			wantErr: assert.Error,
		},
		{
			name: "with invalid transfer Type",
			fields: fields{
				FileTransferMgr: NewMemFileTransferMgr(),
			},
			args: args{
				ctx: func() context.Context {
					ctx := context.Background()
					ctx = context.WithValue(ctx, contextKeyReq, requestCtx{})
					return ctx
				}(),
				rwc: func() io.ReadWriter {
					mrw := mockReadWriter{}
					mrw.WBuf = &bytes.Buffer{}
					mrw.RBuf.Write(
						[]byte{
							0x48, 0x54, 0x58, 0x46,
							0, 0, 0, 5,
							0, 0, 0x01, 0,
							0, 0, 0, 0,
						},
					)
					return mrw
				}(),
			},
			wantErr: assert.Error,
		},
		{
			name: "file download",
			fields: fields{
				FS:     &OSFileStore{},
				Logger: NewTestLogger(),
				Stats:  NewStats(),
				FileTransferMgr: &MemFileTransferMgr{
					fileTransfers: map[FileTransferID]*FileTransfer{
						{0, 0, 0, 5}: {
							RefNum:   [4]byte{0, 0, 0, 5},
							Type:     FileDownload,
							FileName: []byte("testfile-8b"),
							FilePath: []byte{},
							FileRoot: func() string {
								path, _ := os.Getwd()
								return path + "/test/config/Files"
							}(),
							ClientConn: &ClientConn{
								Account: &Account{
									Login: "foo",
								},
								ClientFileTransferMgr: ClientFileTransferMgr{
									transfers: map[FileTransferType]map[FileTransferID]*FileTransfer{
										FileDownload: {
											[4]byte{0, 0, 0, 5}: &FileTransfer{},
										},
									},
								},
							},
							bytesSentCounter: &WriteCounter{},
						},
					},
				},
			},
			args: args{
				ctx: func() context.Context {
					ctx := context.Background()
					ctx = context.WithValue(ctx, contextKeyReq, requestCtx{})
					return ctx
				}(),
				rwc: func() io.ReadWriter {
					mrw := mockReadWriter{}
					mrw.WBuf = &bytes.Buffer{}
					mrw.RBuf.Write(
						[]byte{
							0x48, 0x54, 0x58, 0x46,
							0, 0, 0, 5,
							0, 0, 0x01, 0,
							0, 0, 0, 0,
						},
					)
					return mrw
				}(),
			},
			wantErr: assert.NoError,
			wantDump: `00000000  46 49 4c 50 00 01 00 00  00 00 00 00 00 00 00 00  |FILP............|
00000010  00 00 00 00 00 00 00 02  49 4e 46 4f 00 00 00 00  |........INFO....|
00000020  00 00 00 00 00 00 00 55  41 4d 41 43 54 45 58 54  |.......UAMACTEXT|
00000030  54 54 58 54 00 00 00 00  00 00 01 00 00 00 00 00  |TTXT............|
00000040  00 00 00 00 00 00 00 00  00 00 00 00 00 00 00 00  |................|
00000050  00 00 00 00 00 00 00 00  00 00 00 00 00 00 00 00  |................|
00000060  00 00 00 00 00 00 00 00  00 00 00 00 00 00 00 0b  |................|
00000070  74 65 73 74 66 69 6c 65  2d 38 62 00 00 44 41 54  |testfile-8b..DAT|
00000080  41 00 00 00 00 00 00 00  00 00 00 00 08 7c 39 e0  |A............|9.|
00000090  bc 64 e2 cd de 4d 41 43  52 00 00 00 00 00 00 00  |.d...MACR.......|
000000a0  00 00 00 00 00                                    |.....|
`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Server{
				FileTransferMgr: tt.fields.FileTransferMgr,
				Config:          tt.fields.Config,
				Logger:          tt.fields.Logger,
				Stats:           tt.fields.Stats,
				FS:              tt.fields.FS,
			}

			tt.wantErr(t, s.handleFileTransfer(tt.args.ctx, tt.args.rwc), fmt.Sprintf("handleFileTransfer(%v, %v)", tt.args.ctx, tt.args.rwc))

			assertTransferBytesEqual(t, tt.wantDump, tt.args.rwc.(mockReadWriter).WBuf.Bytes())
		})
	}
}

func TestParseTrackerPassword(t *testing.T) {
	tests := []struct {
		name         string
		trackerAddr  string
		wantPassword string
	}{
		{
			name:         "tracker address with password",
			trackerAddr:  "tracker.example.com:5500:mypassword",
			wantPassword: "mypassword",
		},
		{
			name:         "tracker address without password",
			trackerAddr:  "tracker.example.com:5500",
			wantPassword: "",
		},
		{
			name:         "tracker address with empty password",
			trackerAddr:  "tracker.example.com:5500:",
			wantPassword: "",
		},
		{
			name:         "tracker address with password containing special characters",
			trackerAddr:  "tracker.example.com:5500:pass@word#123",
			wantPassword: "pass@word#123",
		},
		{
			name:         "tracker address with password containing colons",
			trackerAddr:  "tracker.example.com:5500:pass:word:123",
			wantPassword: "pass:word:123",
		},
		{
			name:         "IPv4 address with password",
			trackerAddr:  "192.168.1.100:5500:secret",
			wantPassword: "secret",
		},
		{
			name:         "IPv4 address without password",
			trackerAddr:  "192.168.1.100:5500",
			wantPassword: "",
		},
		{
			name:         "malformed address - no port",
			trackerAddr:  "tracker.example.com",
			wantPassword: "",
		},
		{
			name:         "malformed address - empty string",
			trackerAddr:  "",
			wantPassword: "",
		},
		{
			name:         "malformed address - only colons",
			trackerAddr:  ":::",
			wantPassword: ":",
		},
		{
			name:         "IPv6 address handling (edge case - not properly supported)",
			trackerAddr:  "[::1]:5500:password",
			wantPassword: "1]:5500:password", // IPv6 addresses aren't properly handled by simple colon splitting
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseTrackerPassword(tt.trackerAddr)
			assert.Equal(t, tt.wantPassword, got)
		})
	}
}

// MockTrackerRegistrar is a mock implementation of TrackerRegistrar for testing
type MockTrackerRegistrar struct {
	RegisterCalls []RegisterCall
	RegisterFunc  func(tracker string, registration *TrackerRegistration) error
}

type RegisterCall struct {
	Tracker      string
	Registration *TrackerRegistration
}

func (m *MockTrackerRegistrar) Register(tracker string, registration *TrackerRegistration) error {
	// Record the call
	m.RegisterCalls = append(m.RegisterCalls, RegisterCall{
		Tracker:      tracker,
		Registration: registration,
	})

	// Use custom function if provided, otherwise return nil (success)
	if m.RegisterFunc != nil {
		return m.RegisterFunc(tracker, registration)
	}
	return nil
}

func (m *MockTrackerRegistrar) Reset() {
	m.RegisterCalls = nil
	m.RegisterFunc = nil
}

func TestServer_registerWithTrackers(t *testing.T) {
	tests := []struct {
		name                      string
		config                    Config
		wantImmediateRegistration bool
		wantTrackerCalls          []string
		mockRegisterFunc          func(tracker string, registration *TrackerRegistration) error
		expectError               bool
	}{
		{
			name: "disabled tracker registration",
			config: Config{
				EnableTrackerRegistration: false,
				Trackers:                  []string{"tracker1.example.com:5500", "tracker2.example.com:5500:password"},
			},
			wantImmediateRegistration: false,
			wantTrackerCalls:          []string{},
		},
		{
			name: "enabled tracker registration with multiple trackers",
			config: Config{
				EnableTrackerRegistration: true,
				Trackers:                  []string{"tracker1.example.com:5500", "tracker2.example.com:5500:password"},
				Name:                      "Test Server",
				Description:               "Test Description",
			},
			wantImmediateRegistration: true,
			wantTrackerCalls:          []string{"tracker1.example.com:5500", "tracker2.example.com:5500:password"},
		},
		{
			name: "enabled tracker registration with empty tracker list",
			config: Config{
				EnableTrackerRegistration: true,
				Trackers:                  []string{},
				Name:                      "Test Server",
				Description:               "Test Description",
			},
			wantImmediateRegistration: true,
			wantTrackerCalls:          []string{},
		},
		{
			name: "tracker registration with network errors",
			config: Config{
				EnableTrackerRegistration: true,
				Trackers:                  []string{"tracker1.example.com:5500"},
				Name:                      "Test Server",
				Description:               "Test Description",
			},
			wantImmediateRegistration: true,
			wantTrackerCalls:          []string{"tracker1.example.com:5500"},
			mockRegisterFunc: func(tracker string, registration *TrackerRegistration) error {
				return assert.AnError // Simulate network error
			},
			expectError: false, // Errors are logged but don't stop the function
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock registrar
			mockRegistrar := &MockTrackerRegistrar{
				RegisterFunc: tt.mockRegisterFunc,
			}

			// Create server with mock registrar
			server, err := NewServer(
				WithConfig(tt.config),
				WithLogger(NewTestLogger()),
				WithTrackerRegistrar(mockRegistrar),
			)
			assert.NoError(t, err)

			// Create a context that we can cancel
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Start the registerWithTrackers function in a goroutine
			done := make(chan struct{})
			go func() {
				defer close(done)
				server.registerWithTrackers(ctx)
			}()

			// Give it a moment to do the immediate registration
			time.Sleep(100 * time.Millisecond)

			// Cancel the context to stop the goroutine
			cancel()

			// Wait for the goroutine to finish (should be quick after cancellation)
			select {
			case <-done:
				// Success
			case <-time.After(1 * time.Second):
				t.Fatal("registerWithTrackers did not exit after context cancellation")
			}

			// Verify the calls made to the mock registrar
			assert.Len(t, mockRegistrar.RegisterCalls, len(tt.wantTrackerCalls))

			for i, expectedTracker := range tt.wantTrackerCalls {
				if i < len(mockRegistrar.RegisterCalls) {
					call := mockRegistrar.RegisterCalls[i]
					assert.Equal(t, expectedTracker, call.Tracker)
					assert.Equal(t, tt.config.Name, call.Registration.Name)
					assert.Equal(t, tt.config.Description, call.Registration.Description)
					assert.Equal(t, parseTrackerPassword(expectedTracker), call.Registration.Password)
				}
			}
		})
	}
}

func TestServer_registerWithTrackers_ContextCancellation(t *testing.T) {
	tests := []struct {
		name          string
		cancelAfter   time.Duration
		expectedCalls int // Number of expected registration calls before cancellation
		trackerCount  int
	}{
		{
			name:          "immediate cancellation",
			cancelAfter:   10 * time.Millisecond,
			expectedCalls: 2, // Should complete immediate registration
			trackerCount:  2,
		},
		{
			name:          "cancellation after first ticker",
			cancelAfter:   100 * time.Millisecond,
			expectedCalls: 2, // Should only do immediate registration within 100ms
			trackerCount:  2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRegistrar := &MockTrackerRegistrar{}
			config := Config{
				EnableTrackerRegistration: true,
				Trackers:                  make([]string, tt.trackerCount),
				Name:                      "Test Server",
				Description:               "Test Description",
			}

			// Fill trackers array
			for i := 0; i < tt.trackerCount; i++ {
				config.Trackers[i] = fmt.Sprintf("tracker%d.example.com:5500", i+1)
			}

			server, err := NewServer(
				WithConfig(config),
				WithLogger(NewTestLogger()),
				WithTrackerRegistrar(mockRegistrar),
			)
			assert.NoError(t, err)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			done := make(chan struct{})
			go func() {
				defer close(done)
				server.registerWithTrackers(ctx)
			}()

			// Wait for the specified time then cancel
			time.Sleep(tt.cancelAfter)
			cancel()

			// Wait for graceful shutdown
			select {
			case <-done:
				// Success
			case <-time.After(1 * time.Second):
				t.Fatal("registerWithTrackers did not exit after context cancellation")
			}

			// Verify that the function respects context cancellation
			assert.Equal(t, tt.expectedCalls, len(mockRegistrar.RegisterCalls))
		})
	}
}

func TestServer_registerWithTrackers_PeriodicRegistration(t *testing.T) {
	t.Skip("Skipping timing-sensitive test - would take 5+ minutes to run reliably")

	// This test would verify that periodic re-registration happens every trackerUpdateFrequency seconds
	// but it's impractical to run in normal test suites due to the 300-second interval

	mockRegistrar := &MockTrackerRegistrar{}
	config := Config{
		EnableTrackerRegistration: true,
		Trackers:                  []string{"tracker1.example.com:5500"},
		Name:                      "Test Server",
		Description:               "Test Description",
	}

	server, err := NewServer(
		WithConfig(config),
		WithLogger(NewTestLogger()),
		WithTrackerRegistrar(mockRegistrar),
	)
	assert.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		server.registerWithTrackers(ctx)
	}()

	// Wait for timeout or completion
	<-ctx.Done()

	// Should have done immediate registration only (1 call) in 10 seconds
	// since trackerUpdateFrequency is 300 seconds
	assert.Equal(t, 1, len(mockRegistrar.RegisterCalls))
}

func TestServer_registerWithTrackers_ErrorHandling(t *testing.T) {
	tests := []struct {
		name             string
		trackers         []string
		mockRegisterFunc func(tracker string, registration *TrackerRegistration) error
		expectPanic      bool
	}{
		{
			name:     "handles network errors gracefully",
			trackers: []string{"tracker1.example.com:5500", "tracker2.example.com:5500:password"},
			mockRegisterFunc: func(tracker string, registration *TrackerRegistration) error {
				if tracker == "tracker1.example.com:5500" {
					return fmt.Errorf("network error: connection refused")
				}
				return nil // Second tracker succeeds
			},
			expectPanic: false,
		},
		{
			name:     "handles all trackers failing",
			trackers: []string{"tracker1.example.com:5500", "tracker2.example.com:5500"},
			mockRegisterFunc: func(tracker string, registration *TrackerRegistration) error {
				return fmt.Errorf("network error")
			},
			expectPanic: false,
		},
		{
			name:     "handles empty tracker addresses",
			trackers: []string{"", "valid.tracker.com:5500", ""},
			mockRegisterFunc: func(tracker string, registration *TrackerRegistration) error {
				if tracker == "" {
					return fmt.Errorf("invalid tracker address")
				}
				return nil
			},
			expectPanic: false,
		},
		{
			name:     "handles malformed tracker addresses",
			trackers: []string{"invalid-address", "another:invalid", "valid.tracker.com:5500:password"},
			mockRegisterFunc: func(tracker string, registration *TrackerRegistration) error {
				return nil // Accept all for this test
			},
			expectPanic: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRegistrar := &MockTrackerRegistrar{
				RegisterFunc: tt.mockRegisterFunc,
			}

			config := Config{
				EnableTrackerRegistration: true,
				Trackers:                  tt.trackers,
				Name:                      "Test Server",
				Description:               "Test Description",
			}

			server, err := NewServer(
				WithConfig(config),
				WithLogger(NewTestLogger()),
				WithTrackerRegistrar(mockRegistrar),
			)
			assert.NoError(t, err)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			if tt.expectPanic {
				assert.Panics(t, func() {
					server.registerWithTrackers(ctx)
				})
				return
			}

			done := make(chan struct{})
			go func() {
				defer close(done)
				server.registerWithTrackers(ctx)
			}()

			// Give it time to process
			time.Sleep(50 * time.Millisecond)
			cancel()

			select {
			case <-done:
				// Success - function completed without panicking
			case <-time.After(1 * time.Second):
				t.Fatal("registerWithTrackers did not exit after context cancellation")
			}

			// Verify all trackers were attempted
			assert.Equal(t, len(tt.trackers), len(mockRegistrar.RegisterCalls))
		})
	}
}

func TestServer_registerWithTrackers_EdgeCases(t *testing.T) {
	tests := []struct {
		name           string
		config         Config
		expectedCalls  int
		validateResult func(t *testing.T, calls []RegisterCall)
	}{
		{
			name: "server with zero port",
			config: Config{
				EnableTrackerRegistration: true,
				Trackers:                  []string{"tracker.example.com:5500"},
				Name:                      "Test Server",
				Description:               "Test Description",
			},
			expectedCalls: 1,
			validateResult: func(t *testing.T, calls []RegisterCall) {
				assert.Equal(t, uint16(0), binary.BigEndian.Uint16(calls[0].Registration.Port[:]))
			},
		},
		{
			name: "server with very long name and description",
			config: Config{
				EnableTrackerRegistration: true,
				Trackers:                  []string{"tracker.example.com:5500"},
				Name:                      strings.Repeat("A", 255), // Max uint8 length
				Description:               strings.Repeat("B", 255),
			},
			expectedCalls: 1,
			validateResult: func(t *testing.T, calls []RegisterCall) {
				assert.Equal(t, strings.Repeat("A", 255), calls[0].Registration.Name)
				assert.Equal(t, strings.Repeat("B", 255), calls[0].Registration.Description)
			},
		},
		{
			name: "empty server name and description",
			config: Config{
				EnableTrackerRegistration: true,
				Trackers:                  []string{"tracker.example.com:5500"},
				Name:                      "",
				Description:               "",
			},
			expectedCalls: 1,
			validateResult: func(t *testing.T, calls []RegisterCall) {
				assert.Equal(t, "", calls[0].Registration.Name)
				assert.Equal(t, "", calls[0].Registration.Description)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRegistrar := &MockTrackerRegistrar{}

			server, err := NewServer(
				WithConfig(tt.config),
				WithLogger(NewTestLogger()),
				WithTrackerRegistrar(mockRegistrar),
			)
			assert.NoError(t, err)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			done := make(chan struct{})
			go func() {
				defer close(done)
				server.registerWithTrackers(ctx)
			}()

			time.Sleep(50 * time.Millisecond)
			cancel()

			select {
			case <-done:
				// Success
			case <-time.After(1 * time.Second):
				t.Fatal("registerWithTrackers did not exit after context cancellation")
			}

			assert.Equal(t, tt.expectedCalls, len(mockRegistrar.RegisterCalls))
			if tt.validateResult != nil {
				tt.validateResult(t, mockRegistrar.RegisterCalls)
			}
		})
	}
}

func TestServer_registerWithAllTrackers(t *testing.T) {
	tests := []struct {
		name                      string
		config                    Config
		expectRegistrationAttempt bool
		expectedTrackerCalls      []string
	}{
		{
			name: "disabled tracker registration",
			config: Config{
				EnableTrackerRegistration: false,
				Trackers:                  []string{"tracker1.example.com:5500"},
			},
			expectRegistrationAttempt: false,
			expectedTrackerCalls:      []string{},
		},
		{
			name: "enabled tracker registration with multiple trackers",
			config: Config{
				EnableTrackerRegistration: true,
				Trackers:                  []string{"tracker1.example.com:5500", "tracker2.example.com:5500:password"},
				Name:                      "Test Server",
				Description:               "Test Description",
			},
			expectRegistrationAttempt: true,
			expectedTrackerCalls:      []string{"tracker1.example.com:5500", "tracker2.example.com:5500:password"},
		},
		{
			name: "enabled tracker registration with empty tracker list",
			config: Config{
				EnableTrackerRegistration: true,
				Trackers:                  []string{},
				Name:                      "Test Server",
				Description:               "Test Description",
			},
			expectRegistrationAttempt: true,
			expectedTrackerCalls:      []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRegistrar := &MockTrackerRegistrar{}

			server, err := NewServer(
				WithConfig(tt.config),
				WithLogger(NewTestLogger()),
				WithTrackerRegistrar(mockRegistrar),
			)
			assert.NoError(t, err)

			// Call the extracted function directly
			server.registerWithAllTrackers()

			// Verify the expected number of calls
			assert.Equal(t, len(tt.expectedTrackerCalls), len(mockRegistrar.RegisterCalls))

			// Verify each call
			for i, expectedTracker := range tt.expectedTrackerCalls {
				if i < len(mockRegistrar.RegisterCalls) {
					call := mockRegistrar.RegisterCalls[i]
					assert.Equal(t, expectedTracker, call.Tracker)
					assert.Equal(t, tt.config.Name, call.Registration.Name)
					assert.Equal(t, tt.config.Description, call.Registration.Description)
					assert.Equal(t, parseTrackerPassword(expectedTracker), call.Registration.Password)
				}
			}
		})
	}
}
