# Phase 4: Inventory Tracking (Target: v0.1.0)

## Goal
Implement in-memory inventory state tracking with basic check-in/check-out functionality. This phase creates the first usable version of the system where you can actually track items on a shelf.

## Current State Analysis

### What We Have (Phase 3.1)
- QR code generation and detection working
- Background monitoring with debouncing
- `DetectedQRCode` tracking visible codes with timestamps
- `visibleCodes` map tracking what's currently in camera view
- 8 test items in `testdata/items.json`

### What We Need
Transform "what codes are visible" into "what items are on the shelf and what their state is"

## Design Decisions

### 1. Data Structures

```go
// InventoryItem represents a single item that can be checked in/out
type InventoryItem struct {
    ItemID      string    // Unique item identifier
    ItemName    string    // Human-readable item name
    State       ItemState // Current state: on_shelf, checked_out
    CheckedInAt time.Time // When item was last checked in to shelf
    CheckedOutAt time.Time // When item was last checked out from shelf
}

// ItemState represents the lifecycle state of an inventory item
type ItemState string

const (
    ItemStateOnShelf    ItemState = "on_shelf"    // Item is currently on the shelf
    ItemStateCheckedOut ItemState = "checked_out" // Item has been removed from shelf
)
```

**Key Design Choices:**
- Keep it minimal - only two states for v0.1.0 (no "missing" state yet - that requires person tracking in Phase 6+)
- Track timestamps for both check-in and check-out (useful for history/debugging)
- Use ItemID as the primary key (one item = one entry in inventory)
- Separate from `DetectedQRCode` - that's for QR detection, this is for inventory state

### 2. State Management

Add to `inventoryKeeperKeeper`:
```go
type inventoryKeeperKeeper struct {
    // ... existing fields ...

    // Inventory state
    inventory   map[string]*InventoryItem // Keyed by ItemID
    inventoryMu sync.RWMutex             // Separate lock for inventory vs QR codes
}
```

**Why separate from visibleCodes?**
- `visibleCodes` = transient detection state (what QR codes camera sees right now)
- `inventory` = persistent business state (what items exist and where they are)
- Different lifecycles: QR codes appear/disappear constantly, inventory changes only on check-in/out

### 3. State Transitions

```
┌─────────────┐
│  Unknown    │ (Item not in inventory)
└──────┬──────┘
       │ add_item / return_item
       ▼
┌─────────────┐
│  on_shelf   │ ◄─── QR code visible
└──────┬──────┘
       │ checkout_item / QR code disappeared after grace period
       ▼
┌─────────────┐
│ checked_out │
└──────┬──────┘
       │ return_item / QR code reappears
       └─────► back to on_shelf
```

**State Transition Logic:**
- Manual transitions: `return_item`, `checkout_item` commands
- Automatic transitions: QR appearance → on_shelf, QR disappearance (after grace period) → checked_out
- Implementation approach: Manual commands first (immediately useful), then automatic transitions (completes the working inventory system)

## Implementation Plan

Phase 4 is implemented incrementally, not monolithically:
1. **First:** Manual commands (Steps 1-6) - Immediately usable for inventory tracking
2. **Then:** Automatic QR→inventory sync (Step 7) - Completes the working inventory system

### Step 1: Add Data Structures
**File:** `module.go`
**Changes:**
- Add `ItemState` type and constants
- Add `InventoryItem` struct
- Add inventory fields to `inventoryKeeperKeeper`
- Initialize inventory map in `NewKeeper`

**Tests:** `module_test.go`
- Test inventory initialization (empty map)
- Test struct field presence

### Step 2: Add Item Command
**Command:** `add_item`
**Purpose:** Add an item to the inventory system in "on_shelf" state

```json
{
  "command": "add_item",
  "item_id": "apple-001",
  "item_name": "Honeycrisp Apple"
}
```

**Response:**
```json
{
  "item_id": "apple-001",
  "item_name": "Honeycrisp Apple",
  "state": "on_shelf",
  "checked_in_at": "2026-01-07T10:30:00Z"
}
```

**Business Rules:**
- Item must not already exist (return error if it does)
- Sets state to `on_shelf`
- Sets `CheckedInAt` to current time
- No QR code required (items can be added before QR codes are generated)

**Tests:**
- Add new item successfully
- Add duplicate item (should fail)
- Add with missing item_id (should fail)
- Add with missing item_name (should fail)
- Verify initial state is "on_shelf"
- Verify CheckedInAt timestamp is set

### Step 3: Get Inventory Command
**Command:** `get_inventory`
**Purpose:** List all items in inventory with their current state

```json
{
  "command": "get_inventory"
}
```

**Response:**
```json
{
  "items": [
    {
      "item_id": "apple-001",
      "item_name": "Honeycrisp Apple",
      "state": "on_shelf",
      "checked_in_at": "2026-01-07T10:30:00Z"
    },
    {
      "item_id": "banana-042",
      "item_name": "Organic Banana",
      "state": "checked_out",
      "checked_out_at": "2026-01-07T10:45:00Z"
    }
  ],
  "total_count": 2,
  "on_shelf_count": 1,
  "checked_out_count": 1
}
```

**Optional filters:**
```json
{
  "command": "get_inventory",
  "state": "on_shelf"  // Optional: filter by state
}
```

**Tests:**
- Get empty inventory
- Get inventory with multiple items
- Get inventory with state filter
- Verify counts are accurate

### Step 4: Checkout Item Command
**Command:** `checkout_item`
**Purpose:** Mark an item as removed from shelf

```json
{
  "command": "checkout_item",
  "item_id": "apple-001"
}
```

**Response:**
```json
{
  "item_id": "apple-001",
  "item_name": "Honeycrisp Apple",
  "state": "checked_out",
  "checked_out_at": "2026-01-07T10:45:00Z",
  "previous_state": "on_shelf"
}
```

**Business Rules:**
- Item must exist in inventory (error if not)
- Can checkout from any state (idempotent - checking out a checked-out item is OK)
- Updates `CheckedOutAt` timestamp
- Transitions state to `checked_out`

**Tests:**
- Checkout item from on_shelf state
- Checkout item that's already checked_out (should succeed, update timestamp)
- Checkout non-existent item (should fail)
- Verify state transition
- Verify timestamp update

### Step 5: Return Item Command
**Command:** `return_item`
**Purpose:** Mark an item as returned to shelf

```json
{
  "command": "return_item",
  "item_id": "apple-001"
}
```

**Response:**
```json
{
  "item_id": "apple-001",
  "item_name": "Honeycrisp Apple",
  "state": "on_shelf",
  "checked_in_at": "2026-01-07T11:00:00Z",
  "previous_state": "checked_out"
}
```

**Business Rules:**
- Item must exist in inventory (error if not)
- Can return from any state (idempotent)
- Updates `CheckedInAt` timestamp
- Transitions state to `on_shelf`

**Tests:**
- Return item from checked_out state
- Return item that's already on_shelf (should succeed, update timestamp)
- Return non-existent item (should fail)
- Verify state transition
- Verify timestamp update

### Step 6: Remove Item Command
**Command:** `remove_item`
**Purpose:** Remove an item from the inventory system

```json
{
  "command": "remove_item",
  "item_id": "apple-001"
}
```

**Response:**
```json
{
  "item_id": "apple-001",
  "removed": true,
  "message": "Item removed from inventory"
}
```

**Business Rules:**
- Item must exist (error if not)
- Removes item from inventory map entirely
- No state requirements (can remove from any state)

**Tests:**
- Remove existing item
- Remove non-existent item (should fail)
- Verify item is gone from inventory after removal
- Remove item then try to query it (should not appear in inventory)

### Step 7: Automatic State Transitions
**Purpose:** Connect QR detection events to inventory state changes

**Implementation:**
Hook into the existing QR monitoring system (`monitorQRCodes` goroutine) to detect when codes appear/disappear and update inventory automatically.

**Behavior:**
- When QR code appears (not in `visibleCodes` → added to `visibleCodes`):
  - Look up item by QR data
  - If item exists in inventory and is `checked_out` → transition to `on_shelf`, update `CheckedInAt`
  - Log the automatic check-in
- When QR code disappears (in `visibleCodes` → grace period expires → removed):
  - Look up item by QR data
  - If item exists in inventory and is `on_shelf` → transition to `checked_out`, update `CheckedOutAt`
  - Log the automatic checkout

**Edge Cases:**
- QR code for item not in inventory → log warning, no state change
- Item already in correct state → idempotent, just update timestamp
- Multiple QR codes for same item → use ItemID from QR data, deduplicate

**Tests:**
- QR code appears → item transitions to on_shelf
- QR code disappears → item transitions to checked_out (after grace period)
- QR code for unknown item → no error, logged warning
- QR appears when item already on_shelf → timestamp updated
- QR disappears when item already checked_out → timestamp updated

## Testing Strategy

### Unit Tests (module_test.go)
- Test each command handler in isolation
- Use mock camera and vision service (existing pattern)
- Test all error conditions
- Test state transitions
- Test concurrent access (inventory lock)

### Manual Testing Flow
1. Start viam-server with module
2. Add test items from `testdata/items.json`:
   ```json
   {"command": "add_item", "item_id": "apple-001", "item_name": "Honeycrisp Apple"}
   ```
3. Get inventory - verify items are on_shelf
4. Checkout an item
5. Get inventory - verify counts changed
6. Return an item
7. Get inventory - verify item is back on_shelf
8. Remove an item
9. Get inventory - verify item is gone

### Integration with QR Codes
Phase 4 creates a **complete working inventory system**:
- Items can be added without QR codes (manual tracking)
- Manual commands work independently of QR detection
- Automatic transitions connect QR detection to inventory state (Step 7)
- Full bidirectional integration: manual commands + automatic QR sync

## Files to Modify

1. **module.go**
   - Add `ItemState` type and constants
   - Add `InventoryItem` struct
   - Add inventory fields to `inventoryKeeperKeeper`
   - Add 5 new DoCommand handlers (Steps 2-6)
   - Add automatic state transition logic in `monitorQRCodes` (Step 7)
   - Update `NewKeeper` to initialize inventory

2. **module_test.go**
   - Add tests for all 5 manual commands
   - Add tests for automatic state transitions
   - Add tests for error conditions
   - Add tests for state transitions (manual and automatic)
   - Add test helpers for inventory operations

## Success Criteria

- [ ] All 5 inventory commands implemented and tested
- [ ] Automatic QR→inventory state transitions implemented and tested
- [ ] All tests pass (`make test`)
- [ ] Module builds successfully (`make module`)
- [ ] Can manually add items via DoCommand
- [ ] Can checkout/return items and see state changes (manual)
- [ ] Items automatically transition when QR codes appear/disappear
- [ ] Can query inventory and get accurate counts
- [ ] Can remove items from inventory
- [ ] Thread-safe (can handle concurrent inventory operations)

## Non-Goals for v0.1.0 (Future Phases)

- ❌ Persistent storage (Phase 5 - data capture integration)
- ❌ Transaction history (Phase 5)
- ❌ Person tracking (Phase 6)
- ❌ Theft detection (Phase 7)
- ❌ Web UI (Phase 8)

## Next Steps After v0.1.0

Once Phase 4 is complete (manual commands + automatic transitions), v0.1.0 is ready. The system can track inventory on a shelf with full QR integration.

### Cleanup Interlude (Before Phase 5)
Before proceeding to Phase 5, clean up the codebase:
1. **Rename `inventoryKeeperKeeper`** - Find better name (e.g., `keeper`, `service`, `inventoryService`)
2. **Fix deprecated APIs** - Update to current Viam SDK APIs
3. **Refactor test duplication** - Extract common test setup code into helpers
4. **Remove conversational comments** - Delete comments referencing our discussions, keep only code-relevant docs
5. **Rename test cases to affirmative declarations**:
   - Change "missing camera_name returns error" → "camera_name setting required"
   - Change "duplicate item should fail" → "duplicate items rejected"
   - Apply consistently across all tests

Proceed to Phase 5 (Data Capture & Persistence) to add permanence and history to the system.
