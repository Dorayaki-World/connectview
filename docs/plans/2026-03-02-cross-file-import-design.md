# Cross-File Proto Import Support

## Problem

When proto files import types from other proto files, the VS Code extension fails
because `-I` (include path) doesn't match the import path structure. Users need to
manually configure `connectview.protoRoot`, which is error-prone and the default
(workspace root) often doesn't work.

Example error:
```
user/v1/user.proto: File not found.
order/v1/order.proto:5:1: Import "user/v1/user.proto" was not found or had errors.
```

## Root Cause

The VS Code extension runs protoc with `-I${protoRoot}`, where protoRoot defaults to
the workspace root. If proto files live under `proto/` or `src/proto/`, the import
paths (e.g., `import "user/v1/user.proto"`) don't resolve because protoc looks for
files relative to `-I` paths.

## Design: Auto-detect + Better Error Messages

### Part 1: Import-Based Include Path Detection

**Location:** `vscode/src/protoc.ts` — new `detectImportPaths()` function

Algorithm:
1. Read each discovered `.proto` file and extract `import "..."` statements
2. For each import path, check if it resolves relative to protoRoot or existing include paths
3. If not found, search the workspace for a file matching the import path suffix
4. Compute the required `-I` path: if `user/v1/user.proto` is found at
   `{workspace}/src/proto/user/v1/user.proto`, then `-I {workspace}/src/proto/`
5. Deduplicate and add to include paths

```
import "user/v1/user.proto"
         ↓ search workspace
found at: /workspace/src/proto/user/v1/user.proto
         ↓ strip import path
computed: -I /workspace/src/proto/
```

### Part 2: buf.yaml Detection

Enhance existing `detectIncludePaths()`:
- Search for `buf.yaml` files in the workspace
- Use their parent directory as an include path (buf module root)

### Part 3: Error Message Improvement

When protoc fails with import errors, enhance the error display:
- Parse stderr for "not found" patterns
- Append a hint: "Hint: Set `connectview.protoRoot` to the directory that matches
  your proto import structure, or add paths to `connectview.includePaths`."

## Changes

### `vscode/src/protoc.ts`

1. Add `extractImports(filePath: string): Promise<string[]>` — reads a proto file,
   returns import paths via regex `/^import\s+"([^"]+)";/gm`

2. Add `detectImportPaths(protoRoot, protoFiles, existingIncludes): Promise<string[]>` —
   for each unresolved import, searches workspace for the file and computes include path

3. Modify `compile()` to call `detectImportPaths()` and add results to args

4. Modify `detectIncludePaths()` to also detect `buf.yaml` parent directories

5. Enhance error handling to add hints on import failures

### `vscode/src/protoc.ts` — compile() flow (updated)

```
findProtoFiles(protoRoot)
  → detectIncludePaths(config)       // existing: .buf/, proto_vendor/; new: buf.yaml
  → detectImportPaths(...)           // NEW: scan imports, find missing, compute -I
  → execProtoc(args)
  → on failure: enhanceErrorMessage(stderr)
```

### Test Data

- `testdata/proto/order/v1/order.proto` — imports `user/v1/user.proto` (already added)
- E2E tests for cross-file imports (already added and passing)

## Out of Scope

- Recursive workspace search performance optimization (proto files are typically few)
- Support for proto files outside the workspace
- Replacing protoc with buf for compilation
