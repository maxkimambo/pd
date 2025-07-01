# Logger Migration Guide

This guide explains how to migrate from the dual logger system (`logger.User` and `logger.Op`) to the new unified logger API.

## Overview

The new unified logger provides a simpler, more consistent API while maintaining backward compatibility and preventing nil pointer issues.

### Key Benefits
- **Single API**: Use `logger.*` instead of `logger.User.*` and `logger.Op.*`
- **Always Initialized**: Logger is never nil, preventing runtime panics
- **Structured Logging**: Built-in support for structured fields
- **Flexible Output**: Automatic routing based on log type
- **Environment Variables**: Runtime configuration without code changes

## Migration Steps

### 1. Simple Function Calls

Replace direct logger calls with the new unified API:

```go
// Old API
logger.User.Info("Starting process")
logger.User.Errorf("Failed: %v", err)
logger.Op.Debug("Debug info")

// New API
logger.Info("Starting process")
logger.Errorf("Failed: %v", err)
logger.Debug("Debug info")
```

### 2. Convenience Methods

The convenience methods work the same way:

```go
// Old API
logger.User.Starting("Starting migration")
logger.User.Success("Migration complete")
logger.User.Snapshot("Creating snapshot")

// New API
logger.Starting("Starting migration")
logger.Success("Migration complete") 
logger.Snapshot("Creating snapshot")
```

### 3. Structured Logging

For operational logs with fields:

```go
// Old API
logger.Op.WithFields(map[string]interface{}{
    "disk": diskName,
    "zone": zone,
}).Info("Processing disk")

// New API (Option 1: Direct)
logger.WithFieldsMap(map[string]interface{}{
    "disk": diskName,
    "zone": zone,
}).Info("Processing disk")

// New API (Option 2: Structured)
logger.Info("Processing disk",
    logger.WithFields(map[string]interface{}{
        "disk": diskName,
        "zone": zone,
    })...,
)
```

### 4. Advanced Usage

For cases where you need explicit control over log type:

```go
// Force user-style output (with emoji)
logger.Info("Important message",
    logger.WithLogType(logger.UserLog),
    logger.WithEmoji("⚡"),
)

// Force operational-style output
logger.Info("Technical details",
    logger.WithLogType(logger.OpLog),
)
```

## Environment Variables

The logger supports runtime configuration via environment variables:

### LOG_MODE
Controls the verbosity level:
- `quiet` - Only show errors
- `verbose` - Show all logs including debug
- `debug` - Same as verbose

Example:
```bash
LOG_MODE=quiet pd migrate disk --project my-project
```

### LOG_FORMAT
Controls the output format:
- `text` - Human-readable format (default)
- `json` - JSON format for parsing

Example:
```bash
LOG_FORMAT=json pd migrate disk --project my-project
```

## Breaking Changes

The old API (`logger.User.*` and `logger.Op.*`) has been removed. All code must use the unified logger API.

## Common Patterns

### Command Entry Points
```go
func runCommand(cmd *cobra.Command, args []string) error {
    logger.Starting("Starting process...")
    
    // Process...
    
    logger.Success("Process completed")
    return nil
}
```

### Error Handling
```go
if err != nil {
    logger.Errorf("Operation failed: %v", err)
    return err
}
```

### Progress Updates
```go
logger.Infof("Processing %d items", len(items))
for i, item := range items {
    logger.Infof("Processing item %d/%d: %s", i+1, len(items), item.Name)
    // Process item...
}
logger.Success("All items processed")
```

### Debug Information
```go
logger.Debugf("Configuration: %+v", config)
logger.Debug("Entering critical section",
    logger.WithFields(map[string]interface{}{
        "goroutine": runtime.NumGoroutine(),
        "memory": runtime.MemStats{},
    })...,
)
```

## Testing

When writing tests, the logger is automatically initialized:

```go
func TestMyFunction(t *testing.T) {
    // Logger is ready to use
    logger.Info("Test started")
    
    // Your test code...
}
```

## Migration Checklist

- [ ] Update imports (no changes needed)
- [ ] Replace `logger.User.*` with `logger.*`
- [ ] Replace `logger.Op.*` with `logger.*` 
- [ ] Update structured logging calls if needed
- [ ] Test with different log levels
- [ ] Test with environment variables
- [ ] Remove any logger nil checks (no longer needed)

## Migration Complete

The old API (`logger.User` and `logger.Op`) has been removed. All code now uses the unified logger API, providing a cleaner and more maintainable codebase.