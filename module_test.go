package inventorykeeper

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"reflect"
	"sync"
	"testing"
	"time"

	"go.viam.com/rdk/components/camera"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/resource"
	"go.viam.com/rdk/services/generic"
	"go.viam.com/rdk/services/vision"
	"go.viam.com/rdk/testutils/inject"
	"go.viam.com/rdk/vision/objectdetection"
)

func TestConfigValidate(t *testing.T) {
	t.Run("valid config with camera_name and qr_vision_service", func(t *testing.T) {
		cfg := &Config{
			CameraName:      "shelf-camera",
			QRVisionService: "qr-detector",
		}

		required, optional, err := cfg.Validate("")
		if err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
		if len(required) != 2 {
			t.Errorf("expected 2 required dependencies, got: %d", len(required))
		}
		if required[0] != "shelf-camera" {
			t.Errorf("expected first required dependency 'shelf-camera', got: %s", required[0])
		}
		if required[1] != "qr-detector" {
			t.Errorf("expected second required dependency 'qr-detector', got: %s", required[1])
		}
		if len(optional) != 0 {
			t.Errorf("expected 0 optional dependencies, got: %d", len(optional))
		}
	})

	t.Run("missing camera_name returns error", func(t *testing.T) {
		cfg := &Config{
			QRVisionService: "qr-detector",
		}

		_, _, err := cfg.Validate("")
		if err == nil {
			t.Error("expected error for missing camera_name")
		}
	})

	t.Run("missing qr_vision_service returns error", func(t *testing.T) {
		cfg := &Config{
			CameraName: "shelf-camera",
		}

		_, _, err := cfg.Validate("")
		if err == nil {
			t.Error("expected error for missing qr_vision_service")
		}
	})

	t.Run("negative scan_interval_ms returns error", func(t *testing.T) {
		negativeInterval := -100
		cfg := &Config{
			CameraName:      "shelf-camera",
			QRVisionService: "qr-detector",
			ScanIntervalMs:  &negativeInterval,
		}

		_, _, err := cfg.Validate("")
		if err == nil {
			t.Error("expected error for negative scan_interval_ms")
		}
	})

	t.Run("negative grace_period_ms returns error", func(t *testing.T) {
		negativeGracePeriod := -100
		cfg := &Config{
			CameraName:      "shelf-camera",
			QRVisionService: "qr-detector",
			GracePeriodMs:   &negativeGracePeriod,
		}

		_, _, err := cfg.Validate("")
		if err == nil {
			t.Error("expected error for negative grace_period_ms")
		}
	})
}

func TestDoCommand(t *testing.T) {
	ctx := context.Background()
	logger := logging.NewTestLogger(t)
	logger.SetLevel(logging.ERROR)

	// Explicitly disable background monitoring for this test
	disabledInterval := 0
	cfg := &Config{
		CameraName:      "test-camera",
		QRVisionService: "test-qr-vision",
		ScanIntervalMs:  &disabledInterval,
	}

	mockCam := &inject.Camera{}
	mockVision := inject.NewVisionService("test-qr-vision")

	// Initialize with empty detections to prevent nil pointer panics from background goroutine
	mockVision.DetectionsFromCameraFunc = func(ctx context.Context, cameraName string, extra map[string]interface{}) ([]objectdetection.Detection, error) {
		return []objectdetection.Detection{}, nil
	}

	deps := resource.Dependencies{
		camera.Named("test-camera"):    mockCam,
		vision.Named("test-qr-vision"): mockVision,
	}

	keeper, err := NewKeeper(ctx, deps, resource.NewName(generic.API, "test"), cfg, logger)
	if err != nil {
		t.Fatalf("failed to create keeper: %v", err)
	}
	defer keeper.Close(ctx)

	svc := keeper.(*inventoryKeeperKeeper)

	t.Run("ping command returns success", func(t *testing.T) {
		result, err := svc.DoCommand(ctx, map[string]interface{}{"command": "ping"})
		if err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
		if result["status"] != "ok" {
			t.Errorf("expected status 'ok', got: %v", result["status"])
		}
	})

	t.Run("echo command with message", func(t *testing.T) {
		result, err := svc.DoCommand(ctx, map[string]interface{}{
			"command": "echo",
			"message": "hello world",
		})
		if err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
		if result["message"] != "hello world" {
			t.Errorf("expected message 'hello world', got: %v", result["message"])
		}
	})

	t.Run("echo command without message", func(t *testing.T) {
		result, err := svc.DoCommand(ctx, map[string]interface{}{"command": "echo"})
		if err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
		if result["message"] != "no message provided" {
			t.Errorf("expected default message, got: %v", result["message"])
		}
	})

	t.Run("unknown command returns error", func(t *testing.T) {
		_, err := svc.DoCommand(ctx, map[string]interface{}{"command": "invalid"})
		if err == nil {
			t.Error("expected error for unknown command")
		}
	})

	t.Run("missing command field returns error", func(t *testing.T) {
		_, err := svc.DoCommand(ctx, map[string]interface{}{"something": "else"})
		if err == nil {
			t.Error("expected error for missing command field")
		}
	})

	t.Run("command field not a string returns error", func(t *testing.T) {
		_, err := svc.DoCommand(ctx, map[string]interface{}{"command": 123})
		if err == nil {
			t.Error("expected error for non-string command field")
		}
	})
}

func TestGenerateQR(t *testing.T) {
	ctx := context.Background()
	logger := logging.NewTestLogger(t)
	logger.SetLevel(logging.ERROR)

	// Explicitly disable background monitoring for this test
	disabledInterval := 0
	cfg := &Config{
		CameraName:      "test-camera",
		QRVisionService: "test-qr-vision",
		ScanIntervalMs:  &disabledInterval,
	}

	mockCam := &inject.Camera{}
	mockVision := inject.NewVisionService("test-qr-vision")

	// Initialize with empty detections to prevent nil pointer panics from background goroutine
	mockVision.DetectionsFromCameraFunc = func(ctx context.Context, cameraName string, extra map[string]interface{}) ([]objectdetection.Detection, error) {
		return []objectdetection.Detection{}, nil
	}

	deps := resource.Dependencies{
		camera.Named("test-camera"):    mockCam,
		vision.Named("test-qr-vision"): mockVision,
	}

	keeper, err := NewKeeper(ctx, deps, resource.NewName(generic.API, "test"), cfg, logger)
	if err != nil {
		t.Fatalf("failed to create keeper: %v", err)
	}
	defer keeper.Close(ctx)

	svc := keeper.(*inventoryKeeperKeeper)

	t.Run("generate_qr with valid data", func(t *testing.T) {
		result, err := svc.DoCommand(ctx, map[string]interface{}{
			"command":   "generate_qr",
			"item_id":   "item-001",
			"item_name": "Apple",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Check response has expected fields
		if result["item_id"] != "item-001" {
			t.Errorf("expected item_id 'item-001', got: %v", result["item_id"])
		}
		if result["item_name"] != "Apple" {
			t.Errorf("expected item_name 'Apple', got: %v", result["item_name"])
		}

		// Check QR code is valid base64
		qrCode, ok := result["qr_code"].(string)
		if !ok || qrCode == "" {
			t.Fatal("qr_code missing or not a string")
		}
		if _, err := base64.StdEncoding.DecodeString(qrCode); err != nil {
			t.Errorf("qr_code is not valid base64: %v", err)
		}

		// Check qr_data is valid JSON with correct structure
		qrData, ok := result["qr_data"].(string)
		if !ok {
			t.Fatal("qr_data missing or not a string")
		}

		var itemData ItemQRData
		if err := json.Unmarshal([]byte(qrData), &itemData); err != nil {
			t.Errorf("qr_data is not valid JSON: %v", err)
		}
		if itemData.ItemID != "item-001" {
			t.Errorf("expected qr_data item_id 'item-001', got: %s", itemData.ItemID)
		}
		if itemData.ItemName != "Apple" {
			t.Errorf("expected qr_data item_name 'Apple', got: %s", itemData.ItemName)
		}
	})

	t.Run("generate_qr missing item_id", func(t *testing.T) {
		_, err := svc.DoCommand(ctx, map[string]interface{}{
			"command":   "generate_qr",
			"item_name": "Apple",
		})
		if err == nil {
			t.Error("expected error for missing item_id")
		}
	})

	t.Run("generate_qr missing item_name", func(t *testing.T) {
		_, err := svc.DoCommand(ctx, map[string]interface{}{
			"command": "generate_qr",
			"item_id": "item-001",
		})
		if err == nil {
			t.Error("expected error for missing item_name")
		}
	})

	t.Run("generate_qr empty item_id", func(t *testing.T) {
		_, err := svc.DoCommand(ctx, map[string]interface{}{
			"command":   "generate_qr",
			"item_id":   "",
			"item_name": "Apple",
		})
		if err == nil {
			t.Error("expected error for empty item_id")
		}
	})
}

func TestScanAndCompare(t *testing.T) {
	ctx := context.Background()
	logger := logging.NewTestLogger(t)
	logger.SetLevel(logging.ERROR)

	// Explicitly disable background monitoring for this test
	disabledInterval := 0
	cfg := &Config{
		CameraName:      "test-camera",
		QRVisionService: "test-qr-vision",
		ScanIntervalMs:  &disabledInterval,
	}

	mockCam := &inject.Camera{}
	mockVision := inject.NewVisionService("test-qr-vision")

	// Initialize with empty detections by default to prevent nil pointer in background goroutine
	// Note: The inject package checks if DetectionsFunc is nil, and if so, tries to call the real Service.
	// We need to set DetectionsFunc to non-nil so it uses Detections FromCameraFunc instead.
	mockVision.DetectionsFunc = func(ctx context.Context, img image.Image, extra map[string]interface{}) ([]objectdetection.Detection, error) {
		return []objectdetection.Detection{}, nil
	}
	mockVision.DetectionsFromCameraFunc = func(ctx context.Context, cameraName string, extra map[string]interface{}) ([]objectdetection.Detection, error) {
		return []objectdetection.Detection{}, nil
	}

	deps := resource.Dependencies{
		camera.Named("test-camera"):    mockCam,
		vision.Named("test-qr-vision"): mockVision,
	}

	keeper, err := NewKeeper(ctx, deps, resource.NewName(generic.API, "test"), cfg, logger)
	if err != nil {
		t.Fatalf("failed to create keeper: %v", err)
	}
	defer keeper.Close(ctx)

	svc := keeper.(*inventoryKeeperKeeper)

	// No need to stop monitoring - it never started (ScanIntervalMs = 0)

	t.Run("detects new QR code with ItemQRData", func(t *testing.T) {
		// Create ItemQRData JSON
		qrData := ItemQRData{ItemID: "item-001", ItemName: "Apple"}
		jsonData, _ := json.Marshal(qrData)

		// Set detection behavior for this test
		mockVision.DetectionsFromCameraFunc = func(ctx context.Context, cameraName string, extra map[string]interface{}) ([]objectdetection.Detection, error) {
			return []objectdetection.Detection{
				objectdetection.NewDetection(
					image.Rectangle{Min: image.Point{X: 0, Y: 0}, Max: image.Point{X: 640, Y: 480}}, // Image bounds
					image.Rectangle{Min: image.Point{X: 10, Y: 10}, Max: image.Point{X: 100, Y: 100}}, // Bounding box
					1.0, // Confidence
					string(jsonData),
				),
			}, nil
		}

		// Call scanAndCompare
		svc.scanAndCompare(ctx)

		// Verify code was added to visibleCodes
		svc.monitorMu.Lock()
		defer svc.monitorMu.Unlock()

		if len(svc.visibleCodes) != 1 {
			t.Errorf("expected 1 visible code, got: %d", len(svc.visibleCodes))
		}

		code, ok := svc.visibleCodes[string(jsonData)]
		if !ok {
			t.Fatal("expected code to be in visibleCodes map")
		}

		if code.ItemID != "item-001" {
			t.Errorf("expected ItemID 'item-001', got: %s", code.ItemID)
		}
		if code.ItemName != "Apple" {
			t.Errorf("expected ItemName 'Apple', got: %s", code.ItemName)
		}
		if code.Content != string(jsonData) {
			t.Errorf("expected Content to match JSON data")
		}
	})

	t.Run("detects new QR code with unknown content", func(t *testing.T) {
		// Clear previous state
		svc.monitorMu.Lock()
		svc.visibleCodes = make(map[string]*DetectedQRCode)
		svc.monitorMu.Unlock()

		unknownContent := "https://example.com"

		// Set detection behavior for this test
		mockVision.DetectionsFromCameraFunc = func(ctx context.Context, cameraName string, extra map[string]interface{}) ([]objectdetection.Detection, error) {
			return []objectdetection.Detection{
				objectdetection.NewDetection(
					image.Rectangle{Min: image.Point{X: 0, Y: 0}, Max: image.Point{X: 640, Y: 480}}, // Image bounds
					image.Rectangle{Min: image.Point{X: 10, Y: 10}, Max: image.Point{X: 100, Y: 100}}, // Bounding box
					1.0, // Confidence
					unknownContent,
				),
			}, nil
		}

		// Call scanAndCompare
		svc.scanAndCompare(ctx)

		// Verify code was added
		svc.monitorMu.Lock()
		defer svc.monitorMu.Unlock()

		if len(svc.visibleCodes) != 1 {
			t.Errorf("expected 1 visible code, got: %d", len(svc.visibleCodes))
		}

		code, ok := svc.visibleCodes[unknownContent]
		if !ok {
			t.Fatal("expected code to be in visibleCodes map")
		}

		if code.ItemID != "" {
			t.Errorf("expected empty ItemID for unknown content, got: %s", code.ItemID)
		}
		if code.ItemName != "" {
			t.Errorf("expected empty ItemName for unknown content, got: %s", code.ItemName)
		}
		if code.Content != unknownContent {
			t.Errorf("expected Content '%s', got: %s", unknownContent, code.Content)
		}
	})

	t.Run("detects code disappearance with zero grace period", func(t *testing.T) {
		// Create a new keeper with zero grace period for immediate removal
		zeroGracePeriod := 0
		cfgZeroGrace := &Config{
			CameraName:      "test-camera",
			QRVisionService: "test-qr-vision",
			ScanIntervalMs:  &disabledInterval,
			GracePeriodMs:   &zeroGracePeriod,
		}

		keeperZeroGrace, err := NewKeeper(ctx, deps, resource.NewName(generic.API, "test-zero-grace"), cfgZeroGrace, logger)
		if err != nil {
			t.Fatalf("failed to create keeper: %v", err)
		}
		defer keeperZeroGrace.Close(ctx)

		svcZeroGrace := keeperZeroGrace.(*inventoryKeeperKeeper)

		// Setup: Add a code to visibleCodes
		qrData := ItemQRData{ItemID: "item-002", ItemName: "Banana"}
		jsonData, _ := json.Marshal(qrData)

		svcZeroGrace.monitorMu.Lock()
		svcZeroGrace.visibleCodes = map[string]*DetectedQRCode{
			string(jsonData): {
				Content:  string(jsonData),
				ItemID:   "item-002",
				ItemName: "Banana",
			},
		}
		svcZeroGrace.monitorMu.Unlock()

		// Set detection behavior for this test (return empty to simulate disappearance)
		mockVision.DetectionsFromCameraFunc = func(ctx context.Context, cameraName string, extra map[string]interface{}) ([]objectdetection.Detection, error) {
			return []objectdetection.Detection{}, nil
		}

		// Call scanAndCompare
		svcZeroGrace.scanAndCompare(ctx)

		// Verify code was removed immediately (no grace period)
		svcZeroGrace.monitorMu.Lock()
		defer svcZeroGrace.monitorMu.Unlock()

		if len(svcZeroGrace.visibleCodes) != 0 {
			t.Errorf("expected 0 visible codes after disappearance (zero grace period), got: %d", len(svcZeroGrace.visibleCodes))
		}
	})

	t.Run("handles multiple QR codes", func(t *testing.T) {
		// Clear state
		svc.monitorMu.Lock()
		svc.visibleCodes = make(map[string]*DetectedQRCode)
		svc.monitorMu.Unlock()

		// Create two ItemQRData codes
		qrData1 := ItemQRData{ItemID: "item-001", ItemName: "Apple"}
		jsonData1, _ := json.Marshal(qrData1)

		qrData2 := ItemQRData{ItemID: "item-002", ItemName: "Banana"}
		jsonData2, _ := json.Marshal(qrData2)

		// Set detection behavior for this test (return two detections)
		mockVision.DetectionsFromCameraFunc = func(ctx context.Context, cameraName string, extra map[string]interface{}) ([]objectdetection.Detection, error) {
			return []objectdetection.Detection{
				objectdetection.NewDetection(
					image.Rectangle{Min: image.Point{X: 0, Y: 0}, Max: image.Point{X: 640, Y: 480}}, // Image bounds
					image.Rectangle{Min: image.Point{X: 10, Y: 10}, Max: image.Point{X: 100, Y: 100}}, // Bounding box
					1.0, // Confidence
					string(jsonData1),
				),
				objectdetection.NewDetection(
					image.Rectangle{Min: image.Point{X: 0, Y: 0}, Max: image.Point{X: 640, Y: 480}}, // Image bounds
					image.Rectangle{Min: image.Point{X: 110, Y: 10}, Max: image.Point{X: 200, Y: 100}}, // Bounding box
					1.0, // Confidence
					string(jsonData2),
				),
			}, nil
		}

		// Call scanAndCompare
		svc.scanAndCompare(ctx)

		// Verify both codes were added
		svc.monitorMu.Lock()
		defer svc.monitorMu.Unlock()

		if len(svc.visibleCodes) != 2 {
			t.Errorf("expected 2 visible codes, got: %d", len(svc.visibleCodes))
		}

		code1, ok1 := svc.visibleCodes[string(jsonData1)]
		code2, ok2 := svc.visibleCodes[string(jsonData2)]

		if !ok1 || !ok2 {
			t.Fatal("expected both codes to be in visibleCodes map")
		}

		if code1.ItemID != "item-001" || code1.ItemName != "Apple" {
			t.Error("code1 data mismatch")
		}
		if code2.ItemID != "item-002" || code2.ItemName != "Banana" {
			t.Error("code2 data mismatch")
		}
	})

	t.Run("handles vision service errors gracefully", func(t *testing.T) {
		// Set detection behavior for this test (return an error)
		mockVision.DetectionsFromCameraFunc = func(ctx context.Context, cameraName string, extra map[string]interface{}) ([]objectdetection.Detection, error) {
			return nil, errors.New("vision service unavailable")
		}

		// Call scanAndCompare - should not panic
		svc.scanAndCompare(ctx)

		// State should remain unchanged (empty in this case)
		svc.monitorMu.Lock()
		defer svc.monitorMu.Unlock()

		// visibleCodes should still be empty or unchanged from before the error
	})
}

func TestMonitoringStartBehavior(t *testing.T) {
	ctx := context.Background()
	logger := logging.NewTestLogger(t)
	logger.SetLevel(logging.ERROR)

	t.Run("monitoring starts when ScanIntervalMs is nil", func(t *testing.T) {
		// Track if DetectionsFromCamera was called
		callCount := 0
		var mu sync.Mutex

		mockCam := &inject.Camera{}
		mockVision := inject.NewVisionService("test-qr-vision")

		// Set up DetectionsFunc to make inject package use DetectionsFromCameraFunc
		mockVision.DetectionsFunc = func(ctx context.Context, img image.Image, extra map[string]interface{}) ([]objectdetection.Detection, error) {
			return []objectdetection.Detection{}, nil
		}

		mockVision.DetectionsFromCameraFunc = func(ctx context.Context, cameraName string, extra map[string]interface{}) ([]objectdetection.Detection, error) {
			mu.Lock()
			callCount++
			mu.Unlock()
			return []objectdetection.Detection{}, nil
		}

		deps := resource.Dependencies{
			camera.Named("test-camera"):    mockCam,
			vision.Named("test-qr-vision"): mockVision,
		}

		// Config with nil ScanIntervalMs (should use default 1000ms)
		cfg := &Config{
			CameraName:      "test-camera",
			QRVisionService: "test-qr-vision",
			ScanIntervalMs:  nil,
		}

		keeper, err := NewKeeper(ctx, deps, resource.NewName(generic.API, "test"), cfg, logger)
		if err != nil {
			t.Fatalf("failed to create keeper: %v", err)
		}
		defer keeper.Close(ctx)

		// Wait slightly longer than the default interval to verify monitoring started
		time.Sleep(1200 * time.Millisecond)

		mu.Lock()
		count := callCount
		mu.Unlock()

		if count == 0 {
			t.Error("expected DetectionsFromCamera to be called (monitoring should have started with default interval)")
		}
	})

	t.Run("monitoring disabled when ScanIntervalMs is 0", func(t *testing.T) {
		// Track if DetectionsFromCamera was called
		callCount := 0
		var mu sync.Mutex

		mockCam := &inject.Camera{}
		mockVision := inject.NewVisionService("test-qr-vision")

		mockVision.DetectionsFunc = func(ctx context.Context, img image.Image, extra map[string]interface{}) ([]objectdetection.Detection, error) {
			return []objectdetection.Detection{}, nil
		}

		mockVision.DetectionsFromCameraFunc = func(ctx context.Context, cameraName string, extra map[string]interface{}) ([]objectdetection.Detection, error) {
			mu.Lock()
			callCount++
			mu.Unlock()
			return []objectdetection.Detection{}, nil
		}

		deps := resource.Dependencies{
			camera.Named("test-camera"):    mockCam,
			vision.Named("test-qr-vision"): mockVision,
		}

		// Config with ScanIntervalMs = 0 (explicitly disabled)
		disabledInterval := 0
		cfg := &Config{
			CameraName:      "test-camera",
			QRVisionService: "test-qr-vision",
			ScanIntervalMs:  &disabledInterval,
		}

		keeper, err := NewKeeper(ctx, deps, resource.NewName(generic.API, "test"), cfg, logger)
		if err != nil {
			t.Fatalf("failed to create keeper: %v", err)
		}
		defer keeper.Close(ctx)

		// Wait a bit to ensure monitoring would have started if it were going to
		time.Sleep(200 * time.Millisecond)

		mu.Lock()
		count := callCount
		mu.Unlock()

		if count != 0 {
			t.Errorf("expected DetectionsFromCamera to NOT be called (monitoring should be disabled), but it was called %d times", count)
		}
	})

	t.Run("monitoring starts when ScanIntervalMs is positive", func(t *testing.T) {
		// Track if DetectionsFromCamera was called
		callCount := 0
		var mu sync.Mutex

		mockCam := &inject.Camera{}
		mockVision := inject.NewVisionService("test-qr-vision")

		mockVision.DetectionsFunc = func(ctx context.Context, img image.Image, extra map[string]interface{}) ([]objectdetection.Detection, error) {
			return []objectdetection.Detection{}, nil
		}

		mockVision.DetectionsFromCameraFunc = func(ctx context.Context, cameraName string, extra map[string]interface{}) ([]objectdetection.Detection, error) {
			mu.Lock()
			callCount++
			mu.Unlock()
			return []objectdetection.Detection{}, nil
		}

		deps := resource.Dependencies{
			camera.Named("test-camera"):    mockCam,
			vision.Named("test-qr-vision"): mockVision,
		}

		// Config with custom interval (100ms for faster testing)
		customInterval := 100
		cfg := &Config{
			CameraName:      "test-camera",
			QRVisionService: "test-qr-vision",
			ScanIntervalMs:  &customInterval,
		}

		keeper, err := NewKeeper(ctx, deps, resource.NewName(generic.API, "test"), cfg, logger)
		if err != nil {
			t.Fatalf("failed to create keeper: %v", err)
		}
		defer keeper.Close(ctx)

		// Wait for at least one monitoring cycle (100ms + buffer)
		time.Sleep(150 * time.Millisecond)

		mu.Lock()
		count := callCount
		mu.Unlock()

		if count == 0 {
			t.Error("expected DetectionsFromCamera to be called (monitoring should have started with custom interval)")
		}
	})
}

func TestDebouncingBehavior(t *testing.T) {
	ctx := context.Background()
	logger := logging.NewTestLogger(t)
	logger.SetLevel(logging.ERROR)

	t.Run("code remains visible during grace period", func(t *testing.T) {
		// Explicitly disable background monitoring for this test
		disabledInterval := 0
		gracePeriod := 200 // 200ms grace period
		cfg := &Config{
			CameraName:      "test-camera",
			QRVisionService: "test-qr-vision",
			ScanIntervalMs:  &disabledInterval,
			GracePeriodMs:   &gracePeriod,
		}

		mockCam := &inject.Camera{}
		mockVision := inject.NewVisionService("test-qr-vision")

		// Create ItemQRData JSON
		qrData := ItemQRData{ItemID: "item-001", ItemName: "Apple"}
		jsonData, _ := json.Marshal(qrData)

		// Set detection behavior - initially return detection
		mockVision.DetectionsFunc = func(ctx context.Context, img image.Image, extra map[string]interface{}) ([]objectdetection.Detection, error) {
			return []objectdetection.Detection{}, nil
		}
		mockVision.DetectionsFromCameraFunc = func(ctx context.Context, cameraName string, extra map[string]interface{}) ([]objectdetection.Detection, error) {
			return []objectdetection.Detection{
				objectdetection.NewDetection(
					image.Rectangle{Min: image.Point{X: 0, Y: 0}, Max: image.Point{X: 640, Y: 480}},
					image.Rectangle{Min: image.Point{X: 10, Y: 10}, Max: image.Point{X: 100, Y: 100}},
					1.0,
					string(jsonData),
				),
			}, nil
		}

		deps := resource.Dependencies{
			camera.Named("test-camera"):    mockCam,
			vision.Named("test-qr-vision"): mockVision,
		}

		keeper, err := NewKeeper(ctx, deps, resource.NewName(generic.API, "test"), cfg, logger)
		if err != nil {
			t.Fatalf("failed to create keeper: %v", err)
		}
		defer keeper.Close(ctx)

		svc := keeper.(*inventoryKeeperKeeper)

		// First scan - code appears
		svc.scanAndCompare(ctx)

		// Verify code was added
		svc.monitorMu.Lock()
		if len(svc.visibleCodes) != 1 {
			t.Errorf("expected 1 visible code after first scan, got: %d", len(svc.visibleCodes))
		}
		svc.monitorMu.Unlock()

		// Change mock to return no detections
		mockVision.DetectionsFromCameraFunc = func(ctx context.Context, cameraName string, extra map[string]interface{}) ([]objectdetection.Detection, error) {
			return []objectdetection.Detection{}, nil
		}

		// Second scan - code disappears but within grace period
		svc.scanAndCompare(ctx)

		// Code should still be in visibleCodes but marked as pending removal
		svc.monitorMu.Lock()
		if len(svc.visibleCodes) != 1 {
			t.Errorf("expected 1 visible code during grace period, got: %d", len(svc.visibleCodes))
		}
		code := svc.visibleCodes[string(jsonData)]
		if code == nil {
			t.Fatal("code should still exist during grace period")
		}
		if !code.PendingRemoval {
			t.Error("code should be marked as pending removal")
		}
		svc.monitorMu.Unlock()

		// Wait less than grace period and scan again
		time.Sleep(100 * time.Millisecond)
		svc.scanAndCompare(ctx)

		// Code should still be present (grace period not expired)
		svc.monitorMu.Lock()
		if len(svc.visibleCodes) != 1 {
			t.Errorf("expected 1 visible code (grace period not expired), got: %d", len(svc.visibleCodes))
		}
		svc.monitorMu.Unlock()
	})

	t.Run("code removed after grace period expires", func(t *testing.T) {
		disabledInterval := 0
		gracePeriod := 100 // 100ms grace period for faster test
		cfg := &Config{
			CameraName:      "test-camera",
			QRVisionService: "test-qr-vision",
			ScanIntervalMs:  &disabledInterval,
			GracePeriodMs:   &gracePeriod,
		}

		mockCam := &inject.Camera{}
		mockVision := inject.NewVisionService("test-qr-vision")

		qrData := ItemQRData{ItemID: "item-001", ItemName: "Apple"}
		jsonData, _ := json.Marshal(qrData)

		// Initially return detection
		mockVision.DetectionsFunc = func(ctx context.Context, img image.Image, extra map[string]interface{}) ([]objectdetection.Detection, error) {
			return []objectdetection.Detection{}, nil
		}
		mockVision.DetectionsFromCameraFunc = func(ctx context.Context, cameraName string, extra map[string]interface{}) ([]objectdetection.Detection, error) {
			return []objectdetection.Detection{
				objectdetection.NewDetection(
					image.Rectangle{Min: image.Point{X: 0, Y: 0}, Max: image.Point{X: 640, Y: 480}},
					image.Rectangle{Min: image.Point{X: 10, Y: 10}, Max: image.Point{X: 100, Y: 100}},
					1.0,
					string(jsonData),
				),
			}, nil
		}

		deps := resource.Dependencies{
			camera.Named("test-camera"):    mockCam,
			vision.Named("test-qr-vision"): mockVision,
		}

		keeper, err := NewKeeper(ctx, deps, resource.NewName(generic.API, "test"), cfg, logger)
		if err != nil {
			t.Fatalf("failed to create keeper: %v", err)
		}
		defer keeper.Close(ctx)

		svc := keeper.(*inventoryKeeperKeeper)

		// First scan - code appears
		svc.scanAndCompare(ctx)

		// Change mock to return no detections
		mockVision.DetectionsFromCameraFunc = func(ctx context.Context, cameraName string, extra map[string]interface{}) ([]objectdetection.Detection, error) {
			return []objectdetection.Detection{}, nil
		}

		// Second scan - code disappears
		svc.scanAndCompare(ctx)

		// Wait for grace period to expire
		time.Sleep(150 * time.Millisecond)

		// Third scan - grace period expired, code should be removed
		svc.scanAndCompare(ctx)

		// Code should now be removed
		svc.monitorMu.Lock()
		if len(svc.visibleCodes) != 0 {
			t.Errorf("expected 0 visible codes after grace period, got: %d", len(svc.visibleCodes))
		}
		svc.monitorMu.Unlock()
	})

	t.Run("code reappears during grace period", func(t *testing.T) {
		disabledInterval := 0
		gracePeriod := 200 // 200ms grace period
		cfg := &Config{
			CameraName:      "test-camera",
			QRVisionService: "test-qr-vision",
			ScanIntervalMs:  &disabledInterval,
			GracePeriodMs:   &gracePeriod,
		}

		mockCam := &inject.Camera{}
		mockVision := inject.NewVisionService("test-qr-vision")

		qrData := ItemQRData{ItemID: "item-001", ItemName: "Apple"}
		jsonData, _ := json.Marshal(qrData)

		// Initially return detection
		mockVision.DetectionsFunc = func(ctx context.Context, img image.Image, extra map[string]interface{}) ([]objectdetection.Detection, error) {
			return []objectdetection.Detection{}, nil
		}
		mockVision.DetectionsFromCameraFunc = func(ctx context.Context, cameraName string, extra map[string]interface{}) ([]objectdetection.Detection, error) {
			return []objectdetection.Detection{
				objectdetection.NewDetection(
					image.Rectangle{Min: image.Point{X: 0, Y: 0}, Max: image.Point{X: 640, Y: 480}},
					image.Rectangle{Min: image.Point{X: 10, Y: 10}, Max: image.Point{X: 100, Y: 100}},
					1.0,
					string(jsonData),
				),
			}, nil
		}

		deps := resource.Dependencies{
			camera.Named("test-camera"):    mockCam,
			vision.Named("test-qr-vision"): mockVision,
		}

		keeper, err := NewKeeper(ctx, deps, resource.NewName(generic.API, "test"), cfg, logger)
		if err != nil {
			t.Fatalf("failed to create keeper: %v", err)
		}
		defer keeper.Close(ctx)

		svc := keeper.(*inventoryKeeperKeeper)

		// First scan - code appears
		svc.scanAndCompare(ctx)

		// Change mock to return no detections
		mockVision.DetectionsFromCameraFunc = func(ctx context.Context, cameraName string, extra map[string]interface{}) ([]objectdetection.Detection, error) {
			return []objectdetection.Detection{}, nil
		}

		// Second scan - code disappears
		svc.scanAndCompare(ctx)

		// Verify code is pending removal
		svc.monitorMu.Lock()
		code := svc.visibleCodes[string(jsonData)]
		if code == nil {
			t.Fatal("code should exist during grace period")
		}
		if !code.PendingRemoval {
			t.Error("code should be marked as pending removal")
		}
		svc.monitorMu.Unlock()

		// Wait a bit but less than grace period
		time.Sleep(50 * time.Millisecond)

		// Change mock to return detection again (code reappears)
		mockVision.DetectionsFromCameraFunc = func(ctx context.Context, cameraName string, extra map[string]interface{}) ([]objectdetection.Detection, error) {
			return []objectdetection.Detection{
				objectdetection.NewDetection(
					image.Rectangle{Min: image.Point{X: 0, Y: 0}, Max: image.Point{X: 640, Y: 480}},
					image.Rectangle{Min: image.Point{X: 10, Y: 10}, Max: image.Point{X: 100, Y: 100}},
					1.0,
					string(jsonData),
				),
			}, nil
		}

		// Third scan - code reappears
		svc.scanAndCompare(ctx)

		// Verify code is no longer pending removal
		svc.monitorMu.Lock()
		code = svc.visibleCodes[string(jsonData)]
		if code == nil {
			t.Fatal("code should still exist after reappearing")
		}
		if code.PendingRemoval {
			t.Error("code should not be pending removal after reappearing")
		}
		svc.monitorMu.Unlock()

		// Change mock to no detections again
		mockVision.DetectionsFromCameraFunc = func(ctx context.Context, cameraName string, extra map[string]interface{}) ([]objectdetection.Detection, error) {
			return []objectdetection.Detection{}, nil
		}

		// Wait for original grace period to expire
		time.Sleep(200 * time.Millisecond)

		// Scan - code should NOT be removed (grace period was reset)
		svc.scanAndCompare(ctx)

		svc.monitorMu.Lock()
		if len(svc.visibleCodes) != 1 {
			t.Errorf("expected code to still be present (grace period was reset), got: %d codes", len(svc.visibleCodes))
		}
		svc.monitorMu.Unlock()
	})

	t.Run("grace period of 0 causes immediate removal", func(t *testing.T) {
		disabledInterval := 0
		gracePeriod := 0 // No grace period
		cfg := &Config{
			CameraName:      "test-camera",
			QRVisionService: "test-qr-vision",
			ScanIntervalMs:  &disabledInterval,
			GracePeriodMs:   &gracePeriod,
		}

		mockCam := &inject.Camera{}
		mockVision := inject.NewVisionService("test-qr-vision")

		qrData := ItemQRData{ItemID: "item-001", ItemName: "Apple"}
		jsonData, _ := json.Marshal(qrData)

		// Initially return detection
		mockVision.DetectionsFunc = func(ctx context.Context, img image.Image, extra map[string]interface{}) ([]objectdetection.Detection, error) {
			return []objectdetection.Detection{}, nil
		}
		mockVision.DetectionsFromCameraFunc = func(ctx context.Context, cameraName string, extra map[string]interface{}) ([]objectdetection.Detection, error) {
			return []objectdetection.Detection{
				objectdetection.NewDetection(
					image.Rectangle{Min: image.Point{X: 0, Y: 0}, Max: image.Point{X: 640, Y: 480}},
					image.Rectangle{Min: image.Point{X: 10, Y: 10}, Max: image.Point{X: 100, Y: 100}},
					1.0,
					string(jsonData),
				),
			}, nil
		}

		deps := resource.Dependencies{
			camera.Named("test-camera"):    mockCam,
			vision.Named("test-qr-vision"): mockVision,
		}

		keeper, err := NewKeeper(ctx, deps, resource.NewName(generic.API, "test"), cfg, logger)
		if err != nil {
			t.Fatalf("failed to create keeper: %v", err)
		}
		defer keeper.Close(ctx)

		svc := keeper.(*inventoryKeeperKeeper)

		// First scan - code appears
		svc.scanAndCompare(ctx)

		// Verify code was added
		svc.monitorMu.Lock()
		if len(svc.visibleCodes) != 1 {
			t.Errorf("expected 1 visible code, got: %d", len(svc.visibleCodes))
		}
		svc.monitorMu.Unlock()

		// Change mock to return no detections
		mockVision.DetectionsFromCameraFunc = func(ctx context.Context, cameraName string, extra map[string]interface{}) ([]objectdetection.Detection, error) {
			return []objectdetection.Detection{}, nil
		}

		// Second scan - code disappears and should be immediately removed (grace period = 0)
		svc.scanAndCompare(ctx)

		// Code should be removed immediately
		svc.monitorMu.Lock()
		if len(svc.visibleCodes) != 0 {
			t.Errorf("expected 0 visible codes (no grace period), got: %d", len(svc.visibleCodes))
		}
		svc.monitorMu.Unlock()
	})
}

// ========== Feature 1: Inventory Data Model Tests ==========

func TestInventoryKeeperInitialization(t *testing.T) {
	ctx := context.Background()
	logger := logging.NewTestLogger(t)
	logger.SetLevel(logging.ERROR)

	// Disable background monitoring for this test
	disabledInterval := 0
	cfg := &Config{
		CameraName:      "test-camera",
		QRVisionService: "test-qr-vision",
		ScanIntervalMs:  &disabledInterval,
	}

	mockCam := &inject.Camera{}
	mockVision := inject.NewVisionService("test-qr-vision")

	// Initialize with empty detections
	mockVision.DetectionsFromCameraFunc = func(ctx context.Context, cameraName string, extra map[string]interface{}) ([]objectdetection.Detection, error) {
		return []objectdetection.Detection{}, nil
	}

	deps := resource.Dependencies{
		camera.Named("test-camera"):    mockCam,
		vision.Named("test-qr-vision"): mockVision,
	}

	keeper, err := NewKeeper(ctx, deps, resource.NewName(generic.API, "test"), cfg, logger)
	if err != nil {
		t.Fatalf("failed to create keeper: %v", err)
	}
	defer keeper.Close(ctx)

	svc := keeper.(*inventoryKeeperKeeper)

	t.Run("inventory map is initialized", func(t *testing.T) {
		if svc.inventory == nil {
			t.Error("expected inventory map to be initialized, got nil")
		}
	})

	t.Run("inventory starts empty", func(t *testing.T) {
		svc.inventoryMu.RLock()
		count := len(svc.inventory)
		svc.inventoryMu.RUnlock()

		if count != 0 {
			t.Errorf("expected empty inventory, got %d items", count)
		}
	})

	t.Run("inventory mutex exists", func(t *testing.T) {
		// Test that we can lock and unlock the mutex without panic
		svc.inventoryMu.Lock()
		svc.inventoryMu.Unlock()

		svc.inventoryMu.RLock()
		svc.inventoryMu.RUnlock()
	})
}

func TestInventoryConcurrentAccess(t *testing.T) {
	ctx := context.Background()
	logger := logging.NewTestLogger(t)
	logger.SetLevel(logging.ERROR)

	// Disable background monitoring for this test
	disabledInterval := 0
	cfg := &Config{
		CameraName:      "test-camera",
		QRVisionService: "test-qr-vision",
		ScanIntervalMs:  &disabledInterval,
	}

	mockCam := &inject.Camera{}
	mockVision := inject.NewVisionService("test-qr-vision")

	mockVision.DetectionsFromCameraFunc = func(ctx context.Context, cameraName string, extra map[string]interface{}) ([]objectdetection.Detection, error) {
		return []objectdetection.Detection{}, nil
	}

	deps := resource.Dependencies{
		camera.Named("test-camera"):    mockCam,
		vision.Named("test-qr-vision"): mockVision,
	}

	keeper, err := NewKeeper(ctx, deps, resource.NewName(generic.API, "test"), cfg, logger)
	if err != nil {
		t.Fatalf("failed to create keeper: %v", err)
	}
	defer keeper.Close(ctx)

	svc := keeper.(*inventoryKeeperKeeper)

	t.Run("concurrent reads and writes don't panic", func(t *testing.T) {
		var wg sync.WaitGroup

		// Spawn multiple goroutines doing concurrent reads
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				svc.inventoryMu.RLock()
				_ = len(svc.inventory)
				svc.inventoryMu.RUnlock()
			}()
		}

		// Spawn multiple goroutines doing concurrent writes
		for i := 0; i < 10; i++ {
			wg.Add(1)
			itemID := i
			go func(id int) {
				defer wg.Done()
				svc.inventoryMu.Lock()
				svc.inventory[string(rune(id))] = &InventoryItem{
					ItemID:      string(rune(id)),
					ItemName:    "Test",
					State:       ItemStateOnShelf,
					CheckedInAt: time.Now(),
				}
				svc.inventoryMu.Unlock()
			}(itemID)
		}

		wg.Wait()
	})
}

func TestAddItemCommand(t *testing.T) {
	ctx := context.Background()
	logger := logging.NewTestLogger(t)
	logger.SetLevel(logging.ERROR)

	disabledInterval := 0
	cfg := &Config{
		CameraName:      "test-camera",
		QRVisionService: "test-qr-vision",
		ScanIntervalMs:  &disabledInterval,
	}

	mockCam := &inject.Camera{}
	mockVision := inject.NewVisionService("test-qr-vision")
	mockVision.DetectionsFromCameraFunc = func(ctx context.Context, cameraName string, extra map[string]interface{}) ([]objectdetection.Detection, error) {
		return []objectdetection.Detection{}, nil
	}

	deps := resource.Dependencies{
		camera.Named("test-camera"):    mockCam,
		vision.Named("test-qr-vision"): mockVision,
	}

	keeper, err := NewKeeper(ctx, deps, resource.NewName(generic.API, "test"), cfg, logger)
	if err != nil {
		t.Fatalf("failed to create keeper: %v", err)
	}
	defer keeper.Close(ctx)

	svc := keeper.(*inventoryKeeperKeeper)

	t.Run("new item added successfully", func(t *testing.T) {
		result, err := svc.DoCommand(ctx, map[string]interface{}{
			"command":   "add_item",
			"item_id":   "test-001",
			"item_name": "Test Item",
		})
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		// Validate timestamp separately
		checkedInStr, ok := result["checked_in_at"].(string)
		if !ok {
			t.Fatal("expected checked_in_at to be string")
		}
		checkedInAt, err := time.Parse(time.RFC3339, checkedInStr)
		if err != nil {
			t.Fatalf("failed to parse checked_in_at: %v", err)
		}
		if time.Since(checkedInAt) > 5*time.Second {
			t.Errorf("checked_in_at timestamp not recent: %v", checkedInAt)
		}

		// Copy result and remove timestamp for comparison
		resultWithoutTimestamp := make(map[string]interface{})
		for k, v := range result {
			resultWithoutTimestamp[k] = v
		}
		delete(resultWithoutTimestamp, "checked_in_at")

		expected := map[string]interface{}{
			"item_id":   "test-001",
			"item_name": "Test Item",
			"state":     "on_shelf",
		}
		if !reflect.DeepEqual(resultWithoutTimestamp, expected) {
			t.Errorf("result mismatch:\ngot:  %+v\nwant: %+v", resultWithoutTimestamp, expected)
		}

		// Validate actual inventory state
		svc.inventoryMu.RLock()
		if len(svc.inventory) != 1 {
			t.Errorf("expected 1 item in inventory, got %d", len(svc.inventory))
		}
		item, exists := svc.inventory["test-001"]
		if !exists {
			t.Fatal("item test-001 not found in inventory")
		}
		if item.ItemID != "test-001" || item.ItemName != "Test Item" || item.State != ItemStateOnShelf || item.CheckedInAt.IsZero() {
			t.Errorf("inventory state incorrect: %+v", item)
		}
		svc.inventoryMu.RUnlock()
	})

	t.Run("duplicate items rejected", func(t *testing.T) {
		_, err := svc.DoCommand(ctx, map[string]interface{}{
			"command":   "add_item",
			"item_id":   "test-001",
			"item_name": "Duplicate Item",
		})
		if err == nil {
			t.Error("expected error for duplicate item_id")
		}
	})

	t.Run("item_id field required", func(t *testing.T) {
		_, err := svc.DoCommand(ctx, map[string]interface{}{
			"command":   "add_item",
			"item_name": "No ID Item",
		})
		if err == nil {
			t.Error("expected error for missing item_id")
		}
	})

	t.Run("item_name field required", func(t *testing.T) {
		_, err := svc.DoCommand(ctx, map[string]interface{}{
			"command": "add_item",
			"item_id": "test-002",
		})
		if err == nil {
			t.Error("expected error for missing item_name")
		}
	})
}

func TestGetInventoryCommand(t *testing.T) {
	ctx := context.Background()
	logger := logging.NewTestLogger(t)
	logger.SetLevel(logging.ERROR)

	disabledInterval := 0
	cfg := &Config{
		CameraName:      "test-camera",
		QRVisionService: "test-qr-vision",
		ScanIntervalMs:  &disabledInterval,
	}

	mockCam := &inject.Camera{}
	mockVision := inject.NewVisionService("test-qr-vision")
	mockVision.DetectionsFromCameraFunc = func(ctx context.Context, cameraName string, extra map[string]interface{}) ([]objectdetection.Detection, error) {
		return []objectdetection.Detection{}, nil
	}

	deps := resource.Dependencies{
		camera.Named("test-camera"):    mockCam,
		vision.Named("test-qr-vision"): mockVision,
	}

	keeper, err := NewKeeper(ctx, deps, resource.NewName(generic.API, "test"), cfg, logger)
	if err != nil {
		t.Fatalf("failed to create keeper: %v", err)
	}
	defer keeper.Close(ctx)

	svc := keeper.(*inventoryKeeperKeeper)

	t.Run("empty inventory has zero items and counts", func(t *testing.T) {
		result, err := svc.DoCommand(ctx, map[string]interface{}{
			"command": "get_inventory",
		})
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		items := result["items"].([]interface{})
		if len(items) != 0 {
			t.Errorf("expected empty items array, got %d items", len(items))
		}

		if result["total_count"] != 0 {
			t.Errorf("expected total_count 0, got: %v", result["total_count"])
		}
		if result["on_shelf_count"] != 0 {
			t.Errorf("expected on_shelf_count 0, got: %v", result["on_shelf_count"])
		}
		if result["checked_out_count"] != 0 {
			t.Errorf("expected checked_out_count 0, got: %v", result["checked_out_count"])
		}
	})

	t.Run("inventory lists all items with accurate counts", func(t *testing.T) {
		svc.inventoryMu.Lock()
		svc.inventory["apple-001"] = &InventoryItem{
			ItemID:      "apple-001",
			ItemName:    "Apple",
			State:       ItemStateOnShelf,
			CheckedInAt: time.Now(),
		}
		svc.inventory["banana-001"] = &InventoryItem{
			ItemID:       "banana-001",
			ItemName:     "Banana",
			State:        ItemStateCheckedOut,
			CheckedOutAt: time.Now(),
		}
		svc.inventoryMu.Unlock()

		result, err := svc.DoCommand(ctx, map[string]interface{}{
			"command": "get_inventory",
		})
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		items := result["items"].([]interface{})
		if len(items) != 2 {
			t.Errorf("expected 2 items, got %d", len(items))
		}

		if result["total_count"] != 2 {
			t.Errorf("expected total_count 2, got: %v", result["total_count"])
		}
		if result["on_shelf_count"] != 1 {
			t.Errorf("expected on_shelf_count 1, got: %v", result["on_shelf_count"])
		}
		if result["checked_out_count"] != 1 {
			t.Errorf("expected checked_out_count 1, got: %v", result["checked_out_count"])
		}
	})

	t.Run("state filter returns matching items only", func(t *testing.T) {
		result, err := svc.DoCommand(ctx, map[string]interface{}{
			"command": "get_inventory",
			"state":   "on_shelf",
		})
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		items := result["items"].([]interface{})
		if len(items) != 1 {
			t.Errorf("expected 1 on_shelf item, got %d items", len(items))
		}

		for _, item := range items {
			itemMap := item.(map[string]interface{})
			if itemMap["state"] != "on_shelf" {
				t.Errorf("expected all items to have state 'on_shelf', got: %v", itemMap["state"])
			}
			if itemMap["item_id"] != "apple-001" {
				t.Errorf("expected apple-001, got: %v", itemMap["item_id"])
			}
		}

		if result["total_count"] != 1 {
			t.Errorf("expected total_count 1 (filtered), got: %v", result["total_count"])
		}
	})
}

func TestCheckoutItemCommand(t *testing.T) {
	ctx := context.Background()
	logger := logging.NewTestLogger(t)
	logger.SetLevel(logging.ERROR)

	disabledInterval := 0
	cfg := &Config{
		CameraName:      "test-camera",
		QRVisionService: "test-qr-vision",
		ScanIntervalMs:  &disabledInterval,
	}

	mockCam := &inject.Camera{}
	mockVision := inject.NewVisionService("test-qr-vision")
	mockVision.DetectionsFromCameraFunc = func(ctx context.Context, cameraName string, extra map[string]interface{}) ([]objectdetection.Detection, error) {
		return []objectdetection.Detection{}, nil
	}

	deps := resource.Dependencies{
		camera.Named("test-camera"):    mockCam,
		vision.Named("test-qr-vision"): mockVision,
	}

	keeper, err := NewKeeper(ctx, deps, resource.NewName(generic.API, "test"), cfg, logger)
	if err != nil {
		t.Fatalf("failed to create keeper: %v", err)
	}
	defer keeper.Close(ctx)

	svc := keeper.(*inventoryKeeperKeeper)

	baseTime := time.Date(2026, 1, 7, 10, 0, 0, 0, time.UTC)
	svc.inventoryMu.Lock()
	svc.inventory["test-001"] = &InventoryItem{
		ItemID:      "test-001",
		ItemName:    "Test Item",
		State:       ItemStateOnShelf,
		CheckedInAt: baseTime,
	}
	svc.inventoryMu.Unlock()

	var firstCheckoutTime time.Time

	t.Run("item checked out successfully", func(t *testing.T) {
		result, err := svc.DoCommand(ctx, map[string]interface{}{
			"command": "checkout_item",
			"item_id": "test-001",
		})
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		// Extract and validate timestamp separately
		checkedOutStr, ok := result["checked_out_at"].(string)
		if !ok {
			t.Fatal("expected checked_out_at to be string")
		}
		checkedOutAt, err := time.Parse(time.RFC3339, checkedOutStr)
		if err != nil {
			t.Fatalf("failed to parse checked_out_at: %v", err)
		}
		if time.Since(checkedOutAt) > 5*time.Second {
			t.Errorf("checked_out_at timestamp not recent: %v", checkedOutAt)
		}
		firstCheckoutTime = checkedOutAt

		// Copy result and remove timestamp for comparison
		resultWithoutTimestamp := make(map[string]interface{})
		for k, v := range result {
			resultWithoutTimestamp[k] = v
		}
		delete(resultWithoutTimestamp, "checked_out_at")

		expected := map[string]interface{}{
			"item_id":        "test-001",
			"item_name":      "Test Item",
			"state":          "checked_out",
			"previous_state": "on_shelf",
		}
		if !reflect.DeepEqual(resultWithoutTimestamp, expected) {
			t.Errorf("result mismatch:\ngot:  %+v\nwant: %+v", resultWithoutTimestamp, expected)
		}
	})

	t.Run("repeated checkout updates timestamp", func(t *testing.T) {
		time.Sleep(1100 * time.Millisecond) // Ensure different second for RFC3339 precision
		result, err := svc.DoCommand(ctx, map[string]interface{}{
			"command": "checkout_item",
			"item_id": "test-001",
		})
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		// Extract and validate timestamp is after first checkout
		checkedOutStr, ok := result["checked_out_at"].(string)
		if !ok {
			t.Fatal("expected checked_out_at to be a string")
		}
		checkedOutAt, err := time.Parse(time.RFC3339, checkedOutStr)
		if err != nil {
			t.Fatalf("failed to parse checked_out_at: %v", err)
		}
		if !checkedOutAt.After(firstCheckoutTime) {
			t.Errorf("repeated checkout timestamp %v not after first checkout %v", checkedOutAt, firstCheckoutTime)
		}

		// Copy result and remove timestamp for comparison
		resultWithoutTimestamp := make(map[string]interface{})
		for k, v := range result {
			resultWithoutTimestamp[k] = v
		}
		delete(resultWithoutTimestamp, "checked_out_at")

		expected := map[string]interface{}{
			"item_id":        "test-001",
			"item_name":      "Test Item",
			"state":          "checked_out",
			"previous_state": "checked_out",
		}
		if !reflect.DeepEqual(resultWithoutTimestamp, expected) {
			t.Errorf("result mismatch:\ngot:  %+v\nwant: %+v", resultWithoutTimestamp, expected)
		}
	})

	t.Run("non-existent item rejected", func(t *testing.T) {
		_, err := svc.DoCommand(ctx, map[string]interface{}{
			"command": "checkout_item",
			"item_id": "does-not-exist",
		})
		if err == nil {
			t.Error("expected error for non-existent item")
		}
	})
}

func TestReturnItemCommand(t *testing.T) {
	ctx := context.Background()
	logger := logging.NewTestLogger(t)
	logger.SetLevel(logging.ERROR)

	disabledInterval := 0
	cfg := &Config{
		CameraName:      "test-camera",
		QRVisionService: "test-qr-vision",
		ScanIntervalMs:  &disabledInterval,
	}

	mockCam := &inject.Camera{}
	mockVision := inject.NewVisionService("test-qr-vision")
	mockVision.DetectionsFromCameraFunc = func(ctx context.Context, cameraName string, extra map[string]interface{}) ([]objectdetection.Detection, error) {
		return []objectdetection.Detection{}, nil
	}

	deps := resource.Dependencies{
		camera.Named("test-camera"):    mockCam,
		vision.Named("test-qr-vision"): mockVision,
	}

	keeper, err := NewKeeper(ctx, deps, resource.NewName(generic.API, "test"), cfg, logger)
	if err != nil {
		t.Fatalf("failed to create keeper: %v", err)
	}
	defer keeper.Close(ctx)

	svc := keeper.(*inventoryKeeperKeeper)

	// Setup test item in checked_out state
	baseTime := time.Date(2026, 1, 7, 10, 0, 0, 0, time.UTC)
	svc.inventoryMu.Lock()
	svc.inventory["test-001"] = &InventoryItem{
		ItemID:       "test-001",
		ItemName:     "Test Item",
		State:        ItemStateCheckedOut,
		CheckedOutAt: baseTime,
	}
	svc.inventoryMu.Unlock()

	var firstReturnTime time.Time

	t.Run("item returned successfully", func(t *testing.T) {
		result, err := svc.DoCommand(ctx, map[string]interface{}{
			"command": "return_item",
			"item_id": "test-001",
		})
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		// Extract and validate timestamp separately
		checkedInStr, ok := result["checked_in_at"].(string)
		if !ok {
			t.Fatal("expected checked_in_at to be a string")
		}
		checkedInAt, err := time.Parse(time.RFC3339, checkedInStr)
		if err != nil {
			t.Fatalf("failed to parse checked_in_at: %v", err)
		}
		if time.Since(checkedInAt) > 5*time.Second {
			t.Errorf("checked_in_at timestamp not recent: %v", checkedInAt)
		}
		firstReturnTime = checkedInAt

		// Copy result and remove timestamp for comparison
		resultWithoutTimestamp := make(map[string]interface{})
		for k, v := range result {
			resultWithoutTimestamp[k] = v
		}
		delete(resultWithoutTimestamp, "checked_in_at")

		expected := map[string]interface{}{
			"item_id":        "test-001",
			"item_name":      "Test Item",
			"state":          "on_shelf",
			"previous_state": "checked_out",
		}
		if !reflect.DeepEqual(resultWithoutTimestamp, expected) {
			t.Errorf("result mismatch:\ngot:  %+v\nwant: %+v", resultWithoutTimestamp, expected)
		}

		// Verify actual inventory state has CheckedOutAt cleared
		svc.inventoryMu.RLock()
		item := svc.inventory["test-001"]
		svc.inventoryMu.RUnlock()
		if !item.CheckedOutAt.IsZero() {
			t.Errorf("CheckedOutAt should be zero after return, got: %v", item.CheckedOutAt)
		}
	})

	t.Run("repeated return updates timestamp", func(t *testing.T) {
		time.Sleep(1100 * time.Millisecond) // Ensure different second for RFC3339 precision
		result, err := svc.DoCommand(ctx, map[string]interface{}{
			"command": "return_item",
			"item_id": "test-001",
		})
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		// Extract and validate timestamp is after first return
		checkedInStr, ok := result["checked_in_at"].(string)
		if !ok {
			t.Fatal("expected checked_in_at to be a string")
		}
		checkedInAt, err := time.Parse(time.RFC3339, checkedInStr)
		if err != nil {
			t.Fatalf("failed to parse checked_in_at: %v", err)
		}
		if !checkedInAt.After(firstReturnTime) {
			t.Errorf("repeated return timestamp %v not after first return %v", checkedInAt, firstReturnTime)
		}

		// Copy result and remove timestamp for comparison
		resultWithoutTimestamp := make(map[string]interface{})
		for k, v := range result {
			resultWithoutTimestamp[k] = v
		}
		delete(resultWithoutTimestamp, "checked_in_at")

		expected := map[string]interface{}{
			"item_id":        "test-001",
			"item_name":      "Test Item",
			"state":          "on_shelf",
			"previous_state": "on_shelf",
		}
		if !reflect.DeepEqual(resultWithoutTimestamp, expected) {
			t.Errorf("result mismatch:\ngot:  %+v\nwant: %+v", resultWithoutTimestamp, expected)
		}

		// Verify actual inventory state has CheckedOutAt cleared
		svc.inventoryMu.RLock()
		item := svc.inventory["test-001"]
		svc.inventoryMu.RUnlock()
		if !item.CheckedOutAt.IsZero() {
			t.Errorf("CheckedOutAt should be zero after return, got: %v", item.CheckedOutAt)
		}
	})

	t.Run("non-existent item rejected", func(t *testing.T) {
		_, err := svc.DoCommand(ctx, map[string]interface{}{
			"command": "return_item",
			"item_id": "does-not-exist",
		})
		if err == nil {
			t.Error("expected error for non-existent item")
		}
	})
}

func TestRemoveItemCommand(t *testing.T) {
	ctx := context.Background()
	logger := logging.NewTestLogger(t)
	logger.SetLevel(logging.ERROR)

	disabledInterval := 0
	cfg := &Config{
		CameraName:      "test-camera",
		QRVisionService: "test-qr-vision",
		ScanIntervalMs:  &disabledInterval,
	}

	mockCam := &inject.Camera{}
	mockVision := inject.NewVisionService("test-qr-vision")
	mockVision.DetectionsFromCameraFunc = func(ctx context.Context, cameraName string, extra map[string]interface{}) ([]objectdetection.Detection, error) {
		return []objectdetection.Detection{}, nil
	}

	deps := resource.Dependencies{
		camera.Named("test-camera"):    mockCam,
		vision.Named("test-qr-vision"): mockVision,
	}

	keeper, err := NewKeeper(ctx, deps, resource.NewName(generic.API, "test"), cfg, logger)
	if err != nil {
		t.Fatalf("failed to create keeper: %v", err)
	}
	defer keeper.Close(ctx)

	svc := keeper.(*inventoryKeeperKeeper)

	// Setup test item in inventory
	svc.inventoryMu.Lock()
	svc.inventory["test-001"] = &InventoryItem{
		ItemID:      "test-001",
		ItemName:    "Test Item",
		State:       ItemStateOnShelf,
		CheckedInAt: time.Now(),
	}
	svc.inventoryMu.Unlock()

	t.Run("item removed successfully", func(t *testing.T) {
		result, err := svc.DoCommand(ctx, map[string]interface{}{
			"command": "remove_item",
			"item_id": "test-001",
		})
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		if result["removed"] != true {
			t.Error("expected removed to be true")
		}

		// Verify item is gone
		_, err = svc.DoCommand(ctx, map[string]interface{}{
			"command": "checkout_item",
			"item_id": "test-001",
		})
		if err == nil {
			t.Error("expected error when accessing removed item")
		}
	})

	t.Run("non-existent item rejected", func(t *testing.T) {
		_, err := svc.DoCommand(ctx, map[string]interface{}{
			"command": "remove_item",
			"item_id": "does-not-exist",
		})
		if err == nil {
			t.Error("expected error for non-existent item")
		}
	})
}
