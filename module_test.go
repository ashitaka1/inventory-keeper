package inventorykeeper

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"

	"go.viam.com/rdk/components/camera"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/resource"
	"go.viam.com/rdk/services/generic"
	"go.viam.com/rdk/services/vision"
	"go.viam.com/rdk/testutils/inject"
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
	}

	mockCam := &inject.Camera{}
	mockVision := inject.NewVisionService("test-qr-vision")
	deps := resource.Dependencies{
		camera.Named("test-camera"):        mockCam,
		vision.Named("test-qr-vision"):     mockVision,
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
	}

	mockCam := &inject.Camera{}
	mockVision := inject.NewVisionService("test-qr-vision")
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
