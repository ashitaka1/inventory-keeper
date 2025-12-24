package inventorykeeper

import (
	"context"
	"errors"
	"fmt"

	"go.viam.com/rdk/components/camera"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/resource"
	generic "go.viam.com/rdk/services/generic"
)

var (
	Keeper           = resource.NewModel("viamdemo", "inventory-keeper", "keeper")
	errUnimplemented = errors.New("unimplemented")
)

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

	// Future config fields will be added incrementally as features are implemented:
	// - Vision service for QR detection
	// - Face camera for person detection
	// - ML model service for facial recognition
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

	// Return camera as required dependency
	required := []string{cfg.CameraName}
	return required, nil, nil
}

type inventoryKeeperKeeper struct {
	resource.AlwaysRebuild

	name resource.Name

	logger logging.Logger
	cfg    *Config

	camera camera.Camera // Camera for shelf monitoring

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

	s := &inventoryKeeperKeeper{
		name:       name,
		logger:     logger,
		cfg:        conf,
		camera:     cam,
		cancelCtx:  cancelCtx,
		cancelFunc: cancelFunc,
	}

	logger.Infof("Inventory keeper initialized with camera: %s", conf.CameraName)
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

func (s *inventoryKeeperKeeper) Close(context.Context) error {
	// Put close code here
	s.cancelFunc()
	return nil
}
