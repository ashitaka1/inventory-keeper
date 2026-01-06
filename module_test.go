package inventorykeeper

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"testing"

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
}

func TestDoCommand(t *testing.T) {
	ctx := context.Background()
	logger := logging.NewTestLogger(t)
	cfg := &Config{
		CameraName:      "test-camera",
		QRVisionService: "test-qr-vision",
		ScanIntervalMs:  999999999, // Disable background monitoring for tests
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
	cfg := &Config{
		CameraName:      "test-camera",
		QRVisionService: "test-qr-vision",
		ScanIntervalMs:  999999999, // Disable background monitoring for tests
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
	cfg := &Config{
		CameraName:      "test-camera",
		QRVisionService: "test-qr-vision",
		ScanIntervalMs:  999999999, // Effectively disable background monitoring for tests
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

	// Stop the background monitoring goroutine to prevent race conditions in tests
	svc.cancelFunc()

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

	t.Run("detects code disappearance", func(t *testing.T) {
		// Setup: Add a code to visibleCodes
		qrData := ItemQRData{ItemID: "item-002", ItemName: "Banana"}
		jsonData, _ := json.Marshal(qrData)

		svc.monitorMu.Lock()
		svc.visibleCodes = map[string]*DetectedQRCode{
			string(jsonData): {
				Content:  string(jsonData),
				ItemID:   "item-002",
				ItemName: "Banana",
			},
		}
		svc.monitorMu.Unlock()

		// Set detection behavior for this test (return empty to simulate disappearance)
		mockVision.DetectionsFromCameraFunc = func(ctx context.Context, cameraName string, extra map[string]interface{}) ([]objectdetection.Detection, error) {
			return []objectdetection.Detection{}, nil
		}

		// Call scanAndCompare
		svc.scanAndCompare(ctx)

		// Verify code was removed
		svc.monitorMu.Lock()
		defer svc.monitorMu.Unlock()

		if len(svc.visibleCodes) != 0 {
			t.Errorf("expected 0 visible codes after disappearance, got: %d", len(svc.visibleCodes))
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
