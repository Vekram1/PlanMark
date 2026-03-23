package compile

import "sort"

type AttachedNodeMetadata struct {
	Node       Node
	KnownByKey map[string][]MetadataEntry
	Opaque     []MetadataEntry
}

type MetadataAttachmentResult struct {
	Nodes      []AttachedNodeMetadata
	Unattached []MetadataEntry
}

func AttachMetadataToNodes(nodes []Node, parsed MetadataParseResult) MetadataAttachmentResult {
	result := MetadataAttachmentResult{
		Nodes:      make([]AttachedNodeMetadata, 0, len(nodes)),
		Unattached: make([]MetadataEntry, 0),
	}

	if len(nodes) == 0 {
		result.Unattached = append(result.Unattached, parsed.All...)
		return result
	}

	for _, n := range nodes {
		result.Nodes = append(result.Nodes, AttachedNodeMetadata{
			Node:       n,
			KnownByKey: make(map[string][]MetadataEntry),
			Opaque:     make([]MetadataEntry, 0),
		})
	}

	sortedNodeIndexes := make([]int, len(nodes))
	for i := range nodes {
		sortedNodeIndexes[i] = i
	}
	sort.SliceStable(sortedNodeIndexes, func(i, j int) bool {
		li := nodes[sortedNodeIndexes[i]].Line
		lj := nodes[sortedNodeIndexes[j]].Line
		if li == lj {
			return sortedNodeIndexes[i] < sortedNodeIndexes[j]
		}
		return li < lj
	})

	entries := append([]MetadataEntry(nil), parsed.All...)
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].Line == entries[j].Line {
			if entries[i].Key == entries[j].Key {
				return entries[i].Value < entries[j].Value
			}
			return entries[i].Key < entries[j].Key
		}
		return entries[i].Line < entries[j].Line
	})

	nodeScanIdx := -1
	for _, entry := range entries {
		for nodeScanIdx+1 < len(sortedNodeIndexes) && nodes[sortedNodeIndexes[nodeScanIdx+1]].Line < entry.Line {
			nodeScanIdx++
		}

		if nodeScanIdx < 0 {
			result.Unattached = append(result.Unattached, entry)
			continue
		}
		targetNodeIdx := attachmentOwnerIndex(nodes, sortedNodeIndexes, nodeScanIdx, entry)

		if _, ok := knownMetadataKeys[entry.Key]; ok {
			result.Nodes[targetNodeIdx].KnownByKey[entry.Key] = append(result.Nodes[targetNodeIdx].KnownByKey[entry.Key], entry)
			continue
		}
		result.Nodes[targetNodeIdx].Opaque = append(result.Nodes[targetNodeIdx].Opaque, entry)
	}

	return result
}

func attachmentOwnerIndex(nodes []Node, sortedNodeIndexes []int, nodeScanIdx int, entry MetadataEntry) int {
	targetNodeIdx := sortedNodeIndexes[nodeScanIdx]
	targetNode := nodes[targetNodeIdx]
	if targetNode.Kind != NodeKindCheckbox {
		return targetNodeIdx
	}

	if entry.Indent > targetNode.Indent {
		return targetNodeIdx
	}

	if headingIdx, ok := enclosingHeadingIndex(nodes, sortedNodeIndexes, nodeScanIdx, targetNode.Level); ok {
		return headingIdx
	}

	return targetNodeIdx
}

func enclosingHeadingIndex(nodes []Node, sortedNodeIndexes []int, nodeScanIdx int, currentLevel int) (int, bool) {
	bestIdx := -1
	bestLevel := 0
	for i := nodeScanIdx; i >= 0; i-- {
		idx := sortedNodeIndexes[i]
		node := nodes[idx]
		if node.Kind != NodeKindHeading {
			continue
		}
		if currentLevel > 0 && node.Level >= currentLevel {
			continue
		}
		if bestIdx == -1 || node.Level > bestLevel {
			bestIdx = idx
			bestLevel = node.Level
		}
	}
	if bestIdx == -1 {
		return 0, false
	}
	return bestIdx, true
}
