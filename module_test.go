package inventorykeeper

import (
	"context"
	"testing"

	"go.viam.com/rdk/components/camera"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/resource"
	"go.viam.com/rdk/services/generic"
	"go.viam.com/rdk/testutils/inject"
)

func TestConfigValidate(t *testing.T) {
	t.Run("valid config with camera_name", func(t *testing.T) {
		cfg := &Config{
			CameraName: "shelf-camera",
		}

		required, optional, err := cfg.Validate("")
		if err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
		if len(required) != 1 {
			t.Errorf("expected 1 required dependency, got: %d", len(required))
		}
		if required[0] != "shelf-camera" {
			t.Errorf("expected required dependency 'shelf-camera', got: %s", required[0])
		}
		if len(optional) != 0 {
			t.Errorf("expected 0 optional dependencies, got: %d", len(optional))
		}
	})

	t.Run("missing camera_name returns error", func(t *testing.T) {
		cfg := &Config{}

		_, _, err := cfg.Validate("")
		if err == nil {
			t.Error("expected error for missing camera_name, got nil")
		}
		if err.Error() != "camera_name is required" {
			t.Errorf("expected 'camera_name is required', got: %v", err)
		}
	})
}

func TestDoCommand(t *testing.T) {
	ctx := context.Background()
	logger := logging.NewTestLogger(t)
	cfg := &Config{
		CameraName: "test-camera",
	}

	// Create a mock camera and add to dependencies
	mockCam := &inject.Camera{}
	deps := resource.Dependencies{
		camera.Named("test-camera"): mockCam,
	}

	// Create a keeper instance for testing
	keeper, err := NewKeeper(ctx, deps, resource.NewName(generic.API, "test"), cfg, logger)
	if err != nil {
		t.Fatalf("failed to create keeper: %v", err)
	}
	defer keeper.Close(ctx)

	// Get the service interface
	svc, ok := keeper.(*inventoryKeeperKeeper)
	if !ok {
		t.Fatal("keeper is not of type *inventoryKeeperKeeper")
	}

	t.Run("ping command returns success", func(t *testing.T) {
		cmd := map[string]interface{}{
			"command": "ping",
		}

		result, err := svc.DoCommand(ctx, cmd)
		if err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
		if result["status"] != "ok" {
			t.Errorf("expected status 'ok', got: %v", result["status"])
		}
		if result["message"] != "Inventory keeper is running!" {
			t.Errorf("expected message 'Inventory keeper is running!', got: %v", result["message"])
		}
	})

	t.Run("echo command with message", func(t *testing.T) {
		cmd := map[string]interface{}{
			"command": "echo",
			"message": "hello world",
		}

		result, err := svc.DoCommand(ctx, cmd)
		if err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
		if result["command"] != "echo" {
			t.Errorf("expected command 'echo', got: %v", result["command"])
		}
		if result["message"] != "hello world" {
			t.Errorf("expected message 'hello world', got: %v", result["message"])
		}
		if result["status"] != "success" {
			t.Errorf("expected status 'success', got: %v", result["status"])
		}
	})

	t.Run("echo command without message", func(t *testing.T) {
		cmd := map[string]interface{}{
			"command": "echo",
		}

		result, err := svc.DoCommand(ctx, cmd)
		if err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
		if result["message"] != "no message provided" {
			t.Errorf("expected default message, got: %v", result["message"])
		}
	})

	t.Run("unknown command returns error", func(t *testing.T) {
		cmd := map[string]interface{}{
			"command": "invalid",
		}

		_, err := svc.DoCommand(ctx, cmd)
		if err == nil {
			t.Error("expected error for unknown command, got nil")
		}
		if err.Error() != "unknown command: invalid" {
			t.Errorf("expected 'unknown command: invalid', got: %v", err)
		}
	})

	t.Run("missing command field returns error", func(t *testing.T) {
		cmd := map[string]interface{}{
			"something": "else",
		}

		_, err := svc.DoCommand(ctx, cmd)
		if err == nil {
			t.Error("expected error for missing command field, got nil")
		}
		if err.Error() != "command field is required and must be a string" {
			t.Errorf("expected 'command field is required and must be a string', got: %v", err)
		}
	})

	t.Run("command field not a string returns error", func(t *testing.T) {
		cmd := map[string]interface{}{
			"command": 123,
		}

		_, err := svc.DoCommand(ctx, cmd)
		if err == nil {
			t.Error("expected error for non-string command field, got nil")
		}
		if err.Error() != "command field is required and must be a string" {
			t.Errorf("expected 'command field is required and must be a string', got: %v", err)
		}
	})
}
