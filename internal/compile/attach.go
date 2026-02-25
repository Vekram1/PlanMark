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
		targetNodeIdx := sortedNodeIndexes[nodeScanIdx]

		if _, ok := knownMetadataKeys[entry.Key]; ok {
			result.Nodes[targetNodeIdx].KnownByKey[entry.Key] = append(result.Nodes[targetNodeIdx].KnownByKey[entry.Key], entry)
			continue
		}
		result.Nodes[targetNodeIdx].Opaque = append(result.Nodes[targetNodeIdx].Opaque, entry)
	}

	return result
}
