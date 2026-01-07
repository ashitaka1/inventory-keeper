# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) and Claude agents when working with code in this repository.

## Project Overview

Viam module for automated shelf inventory tracking using QR codes and facial recognition. Built incrementally with test-driven development.

**Module ID**: `viamdemo:inventory-keeper`
**Model**: `viamdemo:inventory-keeper:keeper`
**API**: `rdk:service:generic`
**GitHub**: https://github.com/ashitaka1/inventory-keeper

## Current Status

**Phase 1 Complete** ✅ - Camera Access
- Config field `camera_name` with validation
- Camera accessed from dependencies
- Tests with mock camera
- Module builds successfully

**Phase 2 Complete** ✅ - QR Code Generation
- `generate_qr` DoCommand handler
- Returns base64-encoded PNG QR codes
- ItemQRData struct (item_id, item_name)
- Comprehensive test coverage

**Phase 3 Complete** ✅ - QR Detection & Continuous Monitoring
- Vision service integration with pyzbar
- Config field `qr_vision_service` with validation
- Continuous background monitoring with configurable scan interval
- Pointer-based `scan_interval_ms` config: nil=default 1000ms, 0=disabled, >0=custom interval
- State tracking with DetectedQRCode (FirstSeen, LastSeen timestamps)
- DEBUG logging for QR code appearance/disappearance
- Test fixtures in `testdata/items.json` with `make test-qr` for generating QR codes
- Comprehensive behavioral tests including monitoring start/stop conditions
- Successfully tested with real camera and multiple QR codes

**Phase 3.1 Complete** ✅ - QR Code Debouncing
- Config field `grace_period_ms` with validation (nil=default 2000ms, 0=disabled, >0=custom)
- Grace period prevents false "disappeared" events from temporary detection failures
- Codes marked as `PendingRemoval` when not detected, tracking `DisappearedAt` timestamp
- Only removed after grace period expires without re-detection
- Codes reappearing during grace period reset the timer
- Comprehensive tests for all debouncing scenarios
- Eliminates "flapping" from intermittent camera/lighting issues

## Commands

```bash
make module         # Build module tarball (runs tests first)
make test          # Run all tests
make test-qr       # Generate test QR codes from testdata/items.json
go test -v         # Verbose test output
go test -v -run TestName  # Run specific test

git push           # Push to GitHub
```

## Architecture

### Module Structure
Standard Viam module generated with `viam module generate`:

- **module.go** - Core service (Config, DoCommand, lifecycle)
- **module_test.go** - Tests with mocks
- **cmd/module/main.go** - Entry point
- **cmd/cli/main.go** - Test harness

### Current Config

```go
type Config struct {
    CameraName      string `json:"camera_name"`       // Required
    QRVisionService string `json:"qr_vision_service"` // Required
    ScanIntervalMs  *int   `json:"scan_interval_ms"`  // Optional: nil=1000ms default, 0=disabled, >0=custom
    GracePeriodMs   *int   `json:"grace_period_ms"`   // Optional: nil=2000ms default, 0=no debounce, >0=custom
}
```

More fields added incrementally as features are implemented.

### DoCommand Interface

Current commands:
```json
{"command": "ping"}
{"command": "echo", "message": "hello"}
{"command": "generate_qr", "item_id": "item-001", "item_name": "Apple"}
```

All JSON fields available in `cmd map[string]interface{}`. Use `"command"` for routing, other fields are handler-specific arguments.

### Testing

- Use `inject.Camera` for mocking cameras
- Use `inject.NewVisionService()` for mocking vision services
- Use `logging.NewTestLogger(t)` for logging
- Write tests FIRST (TDD approach)
- All tests must pass before committing

## Development Principles

1. **Incremental**: Add ONE capability at a time
2. **Test-Driven**: Tests before implementation
3. **Config Matches Features**: Only require dependencies we use
4. **Always Working**: Never break existing functionality
5. **Mock Wisely**: Mock complex integrations, not trivial operations
6. **Minimal Data Structures**: Only add fields for features we've directly discussed - no speculative fields during prototyping
7. **Hardware-First**: Test hardware/services in Viam UI before writing code

## Roadmap

1. ✅ **Phase 1: Camera Access** - Access camera from config
2. ✅ **Phase 2: QR Generation** - Generate codes for items
3. ✅ **Phase 3: QR Detection** - Scan codes with vision service, continuous monitoring
4. ✅ **Phase 3.1: Debouncing** - Fix flapping with grace period for disappeared codes
5. **Phase 4: Inventory Tracking** ← **CURRENT** - In-memory inventory state, track items on shelf, basic check-in/check-out

## Future Phases

### Phase 5: Data Capture & Persistence
- Configure Viam data capture service on face camera
- `capture_face` command for manual captures with person labeling
- Collect training dataset (50-100 images per person, varied angles/lighting)
- Data service integration for cloud persistence
- Transaction history storage
- Inventory snapshots and state persistence

### Phase 6: Face Recognition & Person Tracking
- Train ML model using Viam's ML training service
- Config: `face_camera_name`, `ml_model_service_name`
- `recognize_person` command (return identity + confidence)
- Track current person at shelf with timestamps
- User enrollment workflow via webcam
- Handle unknown faces gracefully

### Phase 7: Checkout State Machine
- States: IDLE, PERSON_DETECTED, ITEMS_SCANNED, CHECKOUT_COMPLETE
- State transitions based on person presence + item changes
- Background monitoring with configurable timing:
  - `check_in_delay_seconds` (default: 5s) - grace period before marking item checked in
  - `theft_alert_delay_seconds` (default: 3s) - delay before firing theft alert
- `get_status` command - return current state, person, cart items
- Auto check-in when items return to shelf
- Authorized vs unauthorized removal detection

### Phase 8: Integrations & Alerts
- **Slack Integration** (optional):
  - Config: `slack_webhook_url`
  - Theft alerts with face photo attachments
  - Checkout completion notifications
  - Modular design for future OAuth upgrade
- **StreamDeck Integration** (optional):
  - Config: `streamdeck_name`
  - Display checkout buttons for available items
  - Visual checkout status updates
  - Handle button press events for checkout flow
- **Web App** (React/TypeScript):
  - Real-time inventory view with status badges
  - Item management (add/edit/delete, QR scanner)
  - User enrollment interface
  - Transaction history with filters
  - Alert dashboard with face photos
  - Viam TypeScript SDK integration

### Architecture Notes for Future Phases
- **Event-Driven**: State transitions triggered by QR detection events
- **Modular**: Design for easy extension (webhook → OAuth bot, etc.)
- **Viam-Native**: Use built-in services (vision, ML, data) vs external dependencies
- **State Management**: Item states (on_shelf, checked_out, missing) with timestamps
- **Data Models**: Item, User, Transaction, Alert structs with proper relationships

## Viam Configuration

```json
{
  "modules": [{
    "type": "local",
    "name": "inventory-keeper",
    "executable_path": "/path/to/bin/inventory-keeper"
  }],
  "services": [
    {
      "name": "qr-detector",
      "type": "vision",
      "model": "viam:vision:pyzbar"
    },
    {
      "name": "inventory",
      "namespace": "rdk",
      "type": "generic",
      "model": "viamdemo:inventory-keeper:keeper",
      "attributes": {
        "camera_name": "your-camera-name",
        "qr_vision_service": "qr-detector"
      }
    }
  ]
}
```

## Hardware

- **Dev**: macOS with viam-server, webcams available
- **Target**: Raspberry Pi 5 deployment
- **Vision**: pyzbar QR detector (custom fork in ../viam-qrcode)
- **Faces**: Will use data capture + ML training (Phases 5-6)
