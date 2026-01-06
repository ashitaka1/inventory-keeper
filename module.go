package inventorykeeper

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/skip2/go-qrcode"
	"go.viam.com/rdk/components/camera"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/resource"
	generic "go.viam.com/rdk/services/generic"
	"go.viam.com/rdk/services/vision"
)

var (
	Keeper           = resource.NewModel("viamdemo", "inventory-keeper", "keeper")
	errUnimplemented = errors.New("unimplemented")
)

// ItemQRData represents the data encoded in a QR code for an inventory item
// Fields are added only as features require them - start minimal
type ItemQRData struct {
	ItemID   string `json:"item_id"`
	ItemName string `json:"item_name"`
}

// DetectedQRCode tracks a QR code that's currently visible in the camera view
type DetectedQRCode struct {
	Content   string    // Raw QR code content
	ItemID    string    // Parsed item_id (if content is ItemQRData JSON)
	ItemName  string    // Parsed item_name (if content is ItemQRData JSON)
	FirstSeen time.Time // When this code was first detected
	LastSeen  time.Time // Last time this code was seen
}

func init() {
	resource.RegisterService(generic.API, Keeper,
		resource.Registration[resource.Resource, *Config]{
			Constructor: newInventoryKeeperKeeper,
		},
	)
}

type Config struct {
	// Camera for capturing images of the shelf
	CameraName string `json:"camera_name"`

	// Vision service for QR detection
	QRVisionService string `json:"qr_vision_service"`

	// Scan interval in milliseconds (optional)
	// - nil: defaults to 1000ms, monitoring enabled
	// - 0: monitoring explicitly disabled (useful for tests)
	// - positive value: custom interval, monitoring enabled
	ScanIntervalMs *int `json:"scan_interval_ms,omitempty"`

	// Future config fields will be added incrementally as features are implemented:
	// - Vision service for facial recognition
	// - Face camera for person detection
	// - Optional integrations (streamdeck, slack_webhook_url)
	// - Timing configuration (check_in_delay_seconds, theft_alert_delay_seconds)
}

// Validate ensures all parts of the config are valid and important fields exist.
// Returns three values:
//  1. Required dependencies: other resources that must exist for this resource to work.
//  2. Optional dependencies: other resources that may exist but are not required.
//  3. An error if any Config fields are missing or invalid.
//
// The `path` parameter indicates
// where this resource appears in the machine's JSON configuration
// (for example, "components.0"). You can use it in error messages
// to indicate which resource has a problem.
func (cfg *Config) Validate(path string) ([]string, []string, error) {
	// Validate required camera field
	if cfg.CameraName == "" {
		return nil, nil, errors.New("camera_name is required")
	}

	// Validate required QR vision service field
	if cfg.QRVisionService == "" {
		return nil, nil, errors.New("qr_vision_service is required")
	}

	// Validate scan_interval_ms if provided
	if cfg.ScanIntervalMs != nil && *cfg.ScanIntervalMs < 0 {
		return nil, nil, fmt.Errorf("scan_interval_ms must be non-negative, got: %d", *cfg.ScanIntervalMs)
	}

	// Return both camera and QR vision service as required dependencies
	required := []string{cfg.CameraName, cfg.QRVisionService}
	return required, nil, nil
}

type inventoryKeeperKeeper struct {
	resource.AlwaysRebuild

	name resource.Name

	logger logging.Logger
	cfg    *Config

	camera          camera.Camera  // Camera for shelf monitoring
	qrVisionService vision.Service // Vision service for QR detection

	// QR code monitoring state
	visibleCodes map[string]*DetectedQRCode // Keyed by QR content
	monitorMu    sync.Mutex                  // Protects visibleCodes

	cancelCtx  context.Context
	cancelFunc func()
}

func newInventoryKeeperKeeper(ctx context.Context, deps resource.Dependencies, rawConf resource.Config, logger logging.Logger) (resource.Resource, error) {
	conf, err := resource.NativeConfig[*Config](rawConf)
	if err != nil {
		return nil, err
	}

	return NewKeeper(ctx, deps, rawConf.ResourceName(), conf, logger)

}

func NewKeeper(ctx context.Context, deps resource.Dependencies, name resource.Name, conf *Config, logger logging.Logger) (resource.Resource, error) {

	cancelCtx, cancelFunc := context.WithCancel(context.Background())

	// Get the camera from dependencies
	cam, err := camera.FromDependencies(deps, conf.CameraName)
	if err != nil {
		return nil, fmt.Errorf("failed to get camera %s: %w", conf.CameraName, err)
	}

	// Get the QR vision service from dependencies
	qrVis, err := vision.FromDependencies(deps, conf.QRVisionService)
	if err != nil {
		return nil, fmt.Errorf("failed to get QR vision service %s: %w", conf.QRVisionService, err)
	}

	s := &inventoryKeeperKeeper{
		name:            name,
		logger:          logger,
		cfg:             conf,
		camera:          cam,
		qrVisionService: qrVis,
		visibleCodes:    make(map[string]*DetectedQRCode),
		cancelCtx:       cancelCtx,
		cancelFunc:      cancelFunc,
	}

	// Start background monitoring (only if not explicitly disabled)
	if conf.ScanIntervalMs == nil || *conf.ScanIntervalMs > 0 {
		s.startMonitoring()
	} else {
		logger.Debug("QR code monitoring explicitly disabled (scan_interval_ms=0)")
	}

	logger.Infof("Inventory keeper initialized with camera: %s, QR vision service: %s", conf.CameraName, conf.QRVisionService)
	return s, nil
}

func (s *inventoryKeeperKeeper) Name() resource.Name {
	return s.name
}

func (s *inventoryKeeperKeeper) DoCommand(ctx context.Context, cmd map[string]interface{}) (map[string]interface{}, error) {
	// Get the command type
	cmdType, ok := cmd["command"].(string)
	if !ok {
		return nil, fmt.Errorf("command field is required and must be a string")
	}

	// Route to the appropriate handler based on command type
	switch cmdType {
	case "ping":
		// Health check command
		return map[string]interface{}{
			"status":  "ok",
			"message": "Inventory keeper is running!",
		}, nil

	case "echo":
		// Simple echo command for testing - returns what was sent
		return s.handleEcho(ctx, cmd)

	case "generate_qr":
		// Generate QR code for an inventory item
		return s.handleGenerateQR(ctx, cmd)

	default:
		return nil, fmt.Errorf("unknown command: %s", cmdType)
	}
}

// handleEcho is a simple test command that echoes back the input
func (s *inventoryKeeperKeeper) handleEcho(ctx context.Context, cmd map[string]interface{}) (map[string]interface{}, error) {
	s.logger.Info("Echo command received")

	// Extract the message to echo
	message, ok := cmd["message"]
	if !ok {
		message = "no message provided"
	}

	return map[string]interface{}{
		"command": "echo",
		"message": message,
		"status":  "success",
	}, nil
}

// handleGenerateQR generates a QR code for an inventory item
func (s *inventoryKeeperKeeper) handleGenerateQR(ctx context.Context, cmd map[string]interface{}) (map[string]interface{}, error) {
	s.logger.Info("Generate QR command received")

	// Extract required fields
	itemID, ok := cmd["item_id"].(string)
	if !ok || itemID == "" {
		return nil, errors.New("item_id is required and must be a string")
	}

	itemName, ok := cmd["item_name"].(string)
	if !ok || itemName == "" {
		return nil, errors.New("item_name is required and must be a string")
	}

	// Create QR data structure (minimal - only what we need now)
	qrData := ItemQRData{
		ItemID:   itemID,
		ItemName: itemName,
	}

	// Encode data as JSON
	jsonData, err := json.Marshal(qrData)
	if err != nil {
		return nil, fmt.Errorf("failed to encode QR data: %w", err)
	}

	// Generate QR code (256x256 pixels, medium recovery level)
	qrCode, err := qrcode.Encode(string(jsonData), qrcode.Medium, 256)
	if err != nil {
		return nil, fmt.Errorf("failed to generate QR code: %w", err)
	}

	// Encode as base64 for easy transmission
	qrBase64 := base64.StdEncoding.EncodeToString(qrCode)

	s.logger.Infof("Generated QR code for item: %s", itemID)

	return map[string]interface{}{
		"item_id":   itemID,
		"item_name": itemName,
		"qr_code":   qrBase64,
		"qr_data":   string(jsonData), // Include the encoded data for reference
		"format":    "base64-png",
		"size":      256,
	}, nil
}

// startMonitoring starts the background QR code monitoring loop
func (s *inventoryKeeperKeeper) startMonitoring() {
	// Determine scan interval
	var interval time.Duration
	if s.cfg.ScanIntervalMs == nil {
		// Default to 1 second when not specified
		interval = 1 * time.Second
	} else {
		// Use specified interval (caller ensures this is > 0)
		interval = time.Duration(*s.cfg.ScanIntervalMs) * time.Millisecond
	}

	s.logger.Debugf("Starting QR code monitoring with interval: %v", interval)

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-s.cancelCtx.Done():
				s.logger.Debug("QR code monitoring stopped")
				return
			case <-ticker.C:
				s.scanAndCompare(s.cancelCtx)
			}
		}
	}()
}

// scanAndCompare performs a single scan for QR codes and compares to previous state
func (s *inventoryKeeperKeeper) scanAndCompare(ctx context.Context) {
	// Get detections from vision service
	detections, err := s.qrVisionService.DetectionsFromCamera(ctx, s.cfg.CameraName, nil)
	if err != nil {
		s.logger.Warnf("Failed to scan QR codes: %v", err)
		return
	}

	// Track currently detected codes
	currentCodes := make(map[string]*DetectedQRCode)
	now := time.Now()

	// Process each detection
	for _, detection := range detections {
		content := detection.Label()

		// Try to parse as ItemQRData JSON
		var itemData ItemQRData
		itemID := ""
		itemName := ""
		if err := json.Unmarshal([]byte(content), &itemData); err == nil {
			// Successfully parsed as ItemQRData
			itemID = itemData.ItemID
			itemName = itemData.ItemName
		}

		// Check if this is a new code (appearance)
		s.monitorMu.Lock()
		existingCode, exists := s.visibleCodes[content]
		s.monitorMu.Unlock()

		if !exists {
			// New code appeared
			if itemID != "" {
				s.logger.Debugf("QR code appeared: %s (%s)", itemID, itemName)
			} else {
				s.logger.Debugf("QR code appeared: unknown content - %s", content)
			}
		}

		// Create or update the detected code
		code := &DetectedQRCode{
			Content:  content,
			ItemID:   itemID,
			ItemName: itemName,
			LastSeen: now,
		}

		if existingCode != nil {
			// Preserve FirstSeen timestamp
			code.FirstSeen = existingCode.FirstSeen
		} else {
			code.FirstSeen = now
		}

		currentCodes[content] = code
	}

	// Find codes that disappeared (in visibleCodes but not in currentCodes)
	s.monitorMu.Lock()
	for content, code := range s.visibleCodes {
		if _, stillVisible := currentCodes[content]; !stillVisible {
			// Code disappeared
			if code.ItemID != "" {
				s.logger.Debugf("QR code disappeared: %s (%s)", code.ItemID, code.ItemName)
			} else {
				s.logger.Debugf("QR code disappeared: unknown content - %s", content)
			}
		}
	}

	// Update visible codes map
	s.visibleCodes = currentCodes
	s.monitorMu.Unlock()
}

func (s *inventoryKeeperKeeper) Close(context.Context) error {
	// Put close code here
	s.cancelFunc()
	return nil
}
