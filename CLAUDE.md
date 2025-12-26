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

**Next: Phase 2** - QR Code Generation

## Commands

```bash
make module         # Build module tarball (runs tests first)
make test          # Run all tests
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
    CameraName string `json:"camera_name"` // Required
}
```

More fields added incrementally as features are implemented.

### DoCommand Interface

Current commands:
```json
{"command": "ping"}
{"command": "echo", "message": "hello"}
```

All JSON fields available in `cmd map[string]interface{}`. Use `"command"` for routing, other fields are handler-specific arguments.

### Testing

- Use `inject.Camera` for mocking cameras
- Use `logging.NewTestLogger(t)` for logging
- Write tests FIRST (TDD approach)

## Development Principles

1. **Incremental**: Add ONE capability at a time
2. **Test-Driven**: Tests before implementation
3. **Config Matches Features**: Only require dependencies we use
4. **Always Working**: Never break existing functionality
5. **Mock Wisely**: Mock complex integrations, not trivial operations

## Roadmap to MVP

1. ✅ **Camera Access** - Access camera from config
2. **QR Generation** - Generate codes for items
3. **QR Detection** - Scan codes with vision service
4. **Inventory + Checkout** ← **MVP** - Track items, basic checkout

After MVP: Face recognition, state machine, alerts

## Viam Configuration

```json
{
  "modules": [{
    "type": "local",
    "name": "inventory-keeper",
    "executable_path": "/path/to/bin/inventory-keeper"
  }],
  "services": [{
    "name": "inventory",
    "namespace": "rdk",
    "type": "generic",
    "model": "viamdemo:inventory-keeper:keeper",
    "attributes": {
      "camera_name": "your-camera-name"
    }
  }]
}
```

## Hardware

- **Dev**: macOS with viam-server, webcams available
- **Target**: Raspberry Pi 5 deployment
- **Vision**: Will configure when Phase 3 starts
- **Faces**: Will use data capture + ML training (Phases 5-6)
