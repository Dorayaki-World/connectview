package resolver

import (
	"fmt"
	"maps"

	"github.com/Dorayaki-World/connectview/internal/ir"
)

type Resolver struct {
	root *ir.Root
}

func New(root *ir.Root) *Resolver {
	return &Resolver{root: root}
}

// Resolve walks all RPCs, resolves MessageRef.Resolved pointers,
// resolves Field.ResolvedMessage/ResolvedEnum, and marks IsRecursive.
func (r *Resolver) Resolve() error {
	for _, svc := range r.root.Services {
		for _, rpc := range svc.RPCs {
			if err := r.resolveMessageRef(rpc.Request, nil); err != nil {
				return fmt.Errorf("resolve request for %s: %w", rpc.Name, err)
			}
			if err := r.resolveMessageRef(rpc.Response, nil); err != nil {
				return fmt.Errorf("resolve response for %s: %w", rpc.Name, err)
			}
		}
	}
	return nil
}

// resolveMessageRef resolves a MessageRef and recursively resolves its fields.
// visitedInPath tracks the FQNs of ancestor messages on the current expansion path.
// When the same FQN appears twice on the path, it's a recursive reference.
func (r *Resolver) resolveMessageRef(ref *ir.MessageRef, visitedInPath map[string]bool) error {
	if ref == nil {
		return nil
	}
	msg, ok := r.root.Messages[ref.TypeName]
	if !ok {
		return fmt.Errorf("message not found: %s", ref.TypeName)
	}
	ref.Resolved = msg

	if visitedInPath == nil {
		visitedInPath = make(map[string]bool)
	}

	for _, field := range msg.Fields {
		if field.Type == ir.FieldTypeMessage && field.TypeName != "" && !field.IsMap {
			if visitedInPath[field.TypeName] {
				// This FQN already exists on the current path -> recursive reference
				field.IsRecursive = true
				// Still set ResolvedMessage for type info (but don't expand further)
				field.ResolvedMessage = r.root.Messages[field.TypeName]
				continue
			}
			// Copy visitedInPath and add current message's FQN before recursing
			childPath := copyMap(visitedInPath)
			childPath[ref.TypeName] = true
			childRef := &ir.MessageRef{TypeName: field.TypeName}
			if err := r.resolveMessageRef(childRef, childPath); err != nil {
				return err
			}
			field.ResolvedMessage = childRef.Resolved
		} else if field.Type == ir.FieldTypeEnum {
			field.ResolvedEnum = r.root.Enums[field.TypeName]
		}
	}
	return nil
}

func copyMap(m map[string]bool) map[string]bool {
	cp := make(map[string]bool, len(m))
	maps.Copy(cp, m)
	return cp
}
