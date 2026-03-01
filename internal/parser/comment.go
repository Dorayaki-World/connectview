package parser

import (
	"strings"

	"google.golang.org/protobuf/types/descriptorpb"
)

// extractComment extracts the leading comment from SourceCodeInfo for the given path.
func extractComment(fd *descriptorpb.FileDescriptorProto, path []int32) string {
	if fd.GetSourceCodeInfo() == nil {
		return ""
	}
	for _, loc := range fd.GetSourceCodeInfo().GetLocation() {
		if pathsEqual(loc.GetPath(), path) {
			comment := loc.GetLeadingComments()
			return strings.TrimSpace(comment)
		}
	}
	return ""
}

func pathsEqual(a, b []int32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
