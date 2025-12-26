# Inventory-Keeper Module: Current State & Next Steps

## Current Status Summary

**Phase 1 (Camera Access): ✅ COMPLETE**
- Camera configuration and dependency injection working
- All tests passing (13/13)

**Phase 2 (QR Code Generation): ✅ ALREADY IMPLEMENTED**
- `generate_qr` DoCommand handler fully functional
- Returns base64-encoded PNG QR codes
- Comprehensive test coverage (4 test cases)
- Dependencies installed (github.com/skip2/go-qrcode)

## What's Been Built

### Module Structure
```
inventory-keeper/
├── module.go          (220 lines) - Core service with camera + QR generation
├── module_test.go     (214 lines) - 13 passing tests
├── go.mod             - Dependencies (go-qrcode added)
├── cmd/
│   ├── module/main.go - Viam module entry point
│   └── cli/main.go    - Test harness
└── CLAUDE.md          - Development guide
```

### Current Capabilities

**1. Camera Access (Phase 1)**
- Config field: `camera_name` (required, validated)
- Camera retrieved from dependencies on initialization
- Available to all handlers via `keeper.camera`

**2. QR Code Generation (Phase 2)**
- Command: `generate_qr`
- Input: `{"command": "generate_qr", "item_id": "item-001", "item_name": "Apple"}`
- Output: Base64 PNG (256x256), JSON data, format info
- Validates required fields (item_id, item_name)
- Uses Medium error correction level

**3. Utility Commands**
- `ping` - Health check
- `echo` - Echo messages for testing

### Test Coverage
- ✅ Config validation (valid/invalid cases)
- ✅ DoCommand routing (all commands + error cases)
- ✅ QR generation (success + missing field errors)
- ✅ All 13 tests passing

## Discovery: Phase 2 Already Complete

The CLAUDE.md indicates "Next: Phase 2 - QR Code Generation", but exploration reveals this work is **already done**:

- `handleGenerateQR()` implemented in module.go:111-147
- `ItemQRData` struct defined with item_id and item_name
- Full test suite in module_test.go:135-214
- QR library dependency added to go.mod

**This means the module is actually ready for Phase 3!**

## Roadmap Position

According to CLAUDE.md roadmap:
1. ✅ **Camera Access** - Access camera from config
2. ✅ **QR Generation** - Generate codes for items (DONE!)
3. ⏭️ **QR Detection** - Scan codes with vision service (NEXT)
4. **Inventory + Checkout** ← **MVP** - Track items, basic checkout

## User Decision: Proceed with Phase 3 (QR Detection) ✅

---

# Phase 3: QR Detection with Continuous Monitoring

## Goal
Implement continuous QR code monitoring that tracks which codes are visible and logs appearance/disappearance events at DEBUG level. This provides the foundation for Phase 4's inventory state management.

## Updated Status

**Phase A: Hardware Configuration** ✅ COMPLETE
- pyzbar vision service configured and working
- Vision service validated in Viam UI
- QR codes detected and decoded successfully

**Phase B: Module Integration** ✅ COMPLETE
- `qr_vision_service` config field added
- Vision service integrated into keeper struct
- All tests updated and passing (13/13)

**Phase C: Continuous Monitoring** ⏭️ NEXT
- Background monitoring loop
- QR code state tracking
- DEBUG logging for appearance/disappearance events

## Architecture Overview

### What is Viam's Vision Service?

Viam provides a **vision service API** that abstracts different computer vision capabilities. Vision services can:
- Detect objects
- Classify images
- Find 2D/3D locations
- Decode barcodes/QR codes

The vision service is implemented as a **separate module** that our inventory keeper module will depend on.

### QR Detection: What Module Will We Use?

**Recommended: `viam:vision:pyzbar`**

This is Viam's QR/barcode detection module that uses the pyzbar Python library under the hood. It:
- Detects QR codes in camera images
- Decodes the QR content automatically
- Returns bounding boxes and confidence scores
- Handles multiple QR codes in a single image

**Alternative: `viam:vision:pyzbardetector`** (older, similar functionality)

Both work, but pyzbar is more commonly used in examples.

### How Does Vision Service Integration Work?

**User's Viam Config (machine config):**
```
1. Camera component (webcam) - captures images
2. Vision service (pyzbar module) - detects QR codes
3. Our inventory-keeper module - orchestrates and interprets results
```

**Data flow:**
```
Camera → Vision Service → Our Module
         (pyzbar reads        (interprets
          from camera)         the decoded QR)
```

**Key APIs:**

1. **Vision Service API** (`go.viam.com/rdk/services/vision`)
   - `vision.FromDependencies(deps, name)` - Get vision service from config
   - `vision.DetectionsFromCamera(ctx, cameraName, extra)` - Run detection
   - Returns: `[]objectdetection.Detection`

2. **Detection Object** (`go.viam.com/rdk/vision/objectdetection`)
   - `detection.Label()` - Returns decoded QR content as string
   - `detection.BoundingBox()` - Returns x/y coordinates of QR in image
   - `detection.Score()` - Returns confidence (0.0 to 1.0)

### What Do We Need to Implement?

**Phase C: Continuous Monitoring Loop**

Instead of a command-based approach, we implement continuous background monitoring:

1. **Background Goroutine**
   - Started in `NewKeeper()` after initialization
   - Runs until module Close() is called
   - Periodically polls vision service for QR detections
   - Poll interval: ~500ms-1s (configurable via config field)

2. **QR Code State Tracking**
   - Maintain a map of currently visible QR codes (keyed by content)
   - Compare current detections to previous state on each poll
   - Detect appearance events (new codes)
   - Detect disappearance events (codes no longer visible)

3. **DEBUG Level Logging**
   - Log when QR codes appear: `"QR code detected: item-001 (Apple) at position [x,y]"`
   - Log when QR codes disappear: `"QR code disappeared: item-001 (Apple)"`
   - Log unknown QR codes: `"Unknown QR code detected: https://example.com"`
   - Log errors: `"Vision service error: ..."` (at WARN level)

4. **Data Structures**
   - Track detected codes with: content, item_id (if parsed), item_name (if parsed), last_seen timestamp
   - Use content hash as key for deduplication
   - Clean up disappeared codes after grace period (to handle brief occlusions)

### What Does Our Module NOT Need to Implement?

✅ **QR decoding** - The pyzbar vision service does this
✅ **Image capture** - The vision service reads from camera directly
✅ **Computer vision algorithms** - All in the vision service module
✅ **Bounding box calculation** - Provided by vision service

We only need to:
- Call the vision service API
- Interpret the results
- Format the response for our use case

## Implementation Approach

### Config Changes

Add optional `scan_interval_ms` config field:
```go
type Config struct {
    CameraName      string `json:"camera_name"`
    QRVisionService string `json:"qr_vision_service"`
    ScanIntervalMs  int    `json:"scan_interval_ms"` // Optional, defaults to 1000ms
}
```

### Keeper Struct Changes

Add state tracking fields:
```go
type inventoryKeeperKeeper struct {
    // ... existing fields ...

    // QR monitoring state
    visibleCodes map[string]*DetectedQRCode  // keyed by QR content
    monitorMu    sync.Mutex                   // protects visibleCodes
}

type DetectedQRCode struct {
    Content   string
    ItemID    string  // if parsed as ItemQRData
    ItemName  string  // if parsed as ItemQRData
    FirstSeen time.Time
    LastSeen  time.Time
}
```

### Background Monitoring Loop

Implement `startMonitoring()` method:
```
func (s *inventoryKeeperKeeper) startMonitoring() {
    go func() {
        interval := time.Duration(s.cfg.ScanIntervalMs) * time.Millisecond
        if interval == 0 {
            interval = 1 * time.Second  // default
        }

        ticker := time.NewTicker(interval)
        defer ticker.Stop()

        for {
            select {
            case <-s.cancelCtx.Done():
                return
            case <-ticker.C:
                s.scanAndCompare()
            }
        }
    }()
}

func (s *inventoryKeeperKeeper) scanAndCompare() {
    // Call DetectionsFromCamera
    // Parse detections
    // Compare to s.visibleCodes
    // Log appearance/disappearance at DEBUG level
    // Update state
}
```

### Detection Processing

1. Call `s.qrVisionService.DetectionsFromCamera(ctx, s.cfg.CameraName, nil)`
2. For each detection:
   - Extract `detection.Label()` as QR content
   - Try to parse as `ItemQRData` JSON
   - Create or update entry in `visibleCodes` map
3. Find codes in map but not in current detections (disappeared)
4. Log all changes at DEBUG level

### Logging Examples

**Appearance:**
```
DEBUG: QR code appeared: item-001 (Apple)
DEBUG: QR code appeared: unknown content - https://example.com
```

**Disappearance:**
```
DEBUG: QR code disappeared: item-001 (Apple)
```

**Errors:**
```
WARN: Failed to scan QR codes: failed to get detections from camera: <error>
```

## Testing Approach

### Unit Tests to Add

**Config tests:**
- Optional `scan_interval_ms` field (test default value)

**Monitoring tests:**
- Test `scanAndCompare()` detects new QR codes
- Test `scanAndCompare()` detects disappeared QR codes
- Test ItemQRData parsing (parsed vs unparsed)
- Test logging at DEBUG level (capture log output)
- Test error handling (vision service failure)

**Integration test approach:**
- Mock vision service returns different detections over time
- Verify state map updates correctly
- Verify appropriate DEBUG log messages

### Testing Strategy

Since the monitoring loop runs in background:
1. Call `scanAndCompare()` directly in tests (don't wait for ticker)
2. Mock vision service to return controlled detections
3. Verify state changes and log output
4. Test with multiple scan cycles to verify state transitions

**Note:** We don't need to test the ticker/goroutine mechanics extensively - focus on the detection logic in `scanAndCompare()`.

## Files to Modify

**module.go** - Core service implementation
- Add `sync` and `time` imports
- Add `ScanIntervalMs` to Config (optional field)
- Add `visibleCodes` map and `monitorMu` to keeper struct
- Add `DetectedQRCode` struct definition
- Implement `startMonitoring()` method
- Implement `scanAndCompare()` method
- Call `startMonitoring()` from NewKeeper()
- Add `image` and `objectdetection` imports

**module_test.go** - Tests
- Add tests for `scanAndCompare()` logic
- Test appearance detection
- Test disappearance detection
- Test ItemQRData parsing
- Test logging output

## Dependencies

**No new external packages needed!**
- All vision APIs are in `go.viam.com/rdk` (already our main dependency)
- Vision service, object detection types, and inject mocks all included

## User Configuration

After implementation, users will configure their Viam machine with:

1. **Camera component** - Their webcam or Pi camera
2. **Vision service** - pyzbar module from Viam registry
3. **Our inventory-keeper module** - With both camera_name and vision_name in config

Example config snippet:
```json
{
  "services": [
    {
      "name": "qr-detector",
      "type": "vision",
      "model": "viam:vision:pyzbar"
    },
    {
      "name": "inventory",
      "type": "generic",
      "model": "viamdemo:inventory-keeper:keeper",
      "attributes": {
        "camera_name": "shelf-camera",
        "vision_name": "qr-detector"
      }
    }
  ]
}
```

## Design Considerations

**Why vision service instead of direct QR library?**
- Viam's architecture: vision capabilities as pluggable services
- Allows swapping QR detectors without changing our code
- Vision service handles camera access and image processing
- We focus on business logic (interpreting QR codes as inventory items)

**Why parse as ItemQRData JSON?**
- QR codes contain arbitrary string data
- Our Phase 2 generates JSON-formatted QR codes
- Graceful handling: parse if ItemQRData format, otherwise return raw data
- Enables future scenarios (scanning URLs, other formats, etc.)

**Why return arrays?**
- Multiple QR codes may be visible in camera view
- Each item on shelf gets its own QR code
- Bounding boxes let us know spatial locations

## Implementation Steps

### Step 1: Add Config Field (Optional)
- Add `scan_interval_ms` to Config struct
- Default to 1000ms if not specified
- No validation needed (optional field)

### Step 2: Add State Tracking
- Add `DetectedQRCode` struct definition
- Add `visibleCodes` map to keeper struct
- Add `monitorMu` sync.Mutex to keeper struct
- Initialize map in NewKeeper()

### Step 3: Implement Detection Logic
- Implement `scanAndCompare()` method:
  - Call `DetectionsFromCamera()`
  - Parse each detection (try ItemQRData JSON)
  - Compare to current state
  - Log appearances at DEBUG level
  - Log disappearances at DEBUG level
  - Update state map

### Step 4: Implement Background Loop
- Implement `startMonitoring()` method:
  - Create ticker with configured interval
  - Listen for cancellation context
  - Call `scanAndCompare()` on each tick
- Call `startMonitoring()` from NewKeeper() after initialization

### Step 5: Write Tests
- Test `scanAndCompare()` with mock vision service
- Test appearance detection
- Test disappearance detection
- Test ItemQRData parsing
- Test error handling
- Verify DEBUG logging

### Step 6: Integration Testing
1. Deploy updated module to Viam machine
2. Set log level to DEBUG
3. Hold QR codes up to camera
4. Verify DEBUG logs show appearance/disappearance events
5. Test with multiple codes simultaneously
6. Verify ItemQRData codes are parsed correctly

---

## Why This Approach?

**Benefits for Phase 3:**
- Validates vision service integration works in production
- Provides real-time visibility into detection events
- Tests continuous operation, not just one-shot calls

**Benefits for Phase 4 (MVP):**
- Monitoring loop infrastructure already in place
- State tracking map ready for inventory management
- Just need to add business logic (checkout detection, theft alerts)
- Change detection already working

**Next Phase:** Once continuous monitoring is working, Phase 4 will add:
- Inventory state persistence
- Checkout workflow
- Theft detection (items removed without checkout)
- Alerts/notifications
