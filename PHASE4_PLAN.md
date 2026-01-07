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
- For v0.1.0: Keep it simple with manual commands, automatic transitions deferred to Phase 4.5

## Implementation Plan

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
For v0.1.0, inventory and QR detection are **independent systems**:
- Items can be added without QR codes
- QR codes can be detected without affecting inventory
- Automatic state transitions deferred to Phase 4.5+

## Files to Modify

1. **module.go**
   - Add `ItemState` type and constants
   - Add `InventoryItem` struct
   - Add inventory fields to `inventoryKeeperKeeper`
   - Add 5 new DoCommand handlers
   - Update `NewKeeper` to initialize inventory

2. **module_test.go**
   - Add tests for all 5 new commands
   - Add tests for error conditions
   - Add tests for state transitions
   - Add test helpers for inventory operations

## Success Criteria

- [ ] All 5 inventory commands implemented and tested
- [ ] All tests pass (`make test`)
- [ ] Module builds successfully (`make module`)
- [ ] Can manually add items via DoCommand
- [ ] Can checkout/return items and see state changes
- [ ] Can query inventory and get accurate counts
- [ ] Can remove items from inventory
- [ ] Thread-safe (can handle concurrent inventory operations)

## Non-Goals for v0.1.0 (Future Work)

- ❌ Automatic state transitions based on QR detection (Phase 4.5+)
- ❌ Persistent storage (Phase 5 - data capture integration)
- ❌ Transaction history (Phase 5)
- ❌ Person tracking (Phase 6)
- ❌ Theft detection (Phase 7)
- ❌ Web UI (Phase 8)

## Next Steps After v0.1.0

Once Phase 4 is complete, good candidates for Phase 4.5:
1. **Automatic State Transitions:** Connect QR detection to inventory
   - When QR code appears → auto return if item is checked_out
   - When QR code disappears → auto checkout if item is on_shelf
2. **Bulk Operations:** Add multiple items at once
3. **Inventory Stats:** More detailed analytics and reporting

Or proceed directly to Phase 5 (Data Capture & Persistence) to add permanence to the system.
