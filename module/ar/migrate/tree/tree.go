package tree

import (
	"fmt"
	"sort"
	"strings"

	"harness/module/ar/migrate/types"
)

func TransformToTree(files []types.File) *types.TreeNode {
	// Create root node
	root := types.TreeNode{
		Name:     "root",
		Key:      "/",
		Children: []types.TreeNode{},
		IsLeaf:   false,
	}

	// Process each file and add it to the tree
	for _, file := range files {
		// Skip folders if they exist in the list
		if file.Folder {
			continue
		}

		// Split the URI into path components
		uriPath := strings.TrimPrefix(file.Uri, "/")
		pathComponents := strings.Split(uriPath, "/")

		// Start at the root node
		currentNode := &root

		// Traverse the tree, creating nodes as needed for directories
		for i, component := range pathComponents {
			// Skip empty components
			if component == "" {
				continue
			}

			// Is this the last component (file name)?
			isLast := i == len(pathComponents)-1

			// Look for an existing child with this name
			found := false
			childIndex := -1

			for j, child := range currentNode.Children {
				if child.Name == component {
					found = true
					childIndex = j
					break
				}
			}

			// If we found a matching child, descend into it
			if found {
				// If it's a leaf node but we're not at the last component, something's wrong
				// This shouldn't happen in a well-formed file system, but we check just in case
				if currentNode.Children[childIndex].IsLeaf && !isLast {
					// Skip this file as there's a conflict
					break
				}

				// Move to the existing child node
				currentNode = &currentNode.Children[childIndex]
			} else {
				// Create a new node
				newNode := types.TreeNode{
					Name:     component,
					Key:      currentNode.Key + component + "/",
					Children: []types.TreeNode{},
					IsLeaf:   isLast, // It's a leaf if it's the last component
				}

				// If this is the file (last component), add the file data
				if isLast {
					newNode.File = &file
				}

				// Add the new node to the current node's children
				currentNode.Children = append(currentNode.Children, newNode)

				// Move to the new node
				currentNode = &currentNode.Children[len(currentNode.Children)-1]
			}
		}
	}

	// Sort the tree nodes (optional, for consistent display)
	sortTreeNodes(&root)

	return &root
}

// sortTreeNodes sorts the children of a TreeNode alphabetically
// Directories come before files and nodes are sorted by name
func sortTreeNodes(node *types.TreeNode) {
	// Sort the current node's children
	sort.Slice(node.Children, func(i, j int) bool {
		// If one is a directory and the other is a file, directories come first
		if !node.Children[i].IsLeaf && node.Children[j].IsLeaf {
			return true
		}
		if node.Children[i].IsLeaf && !node.Children[j].IsLeaf {
			return false
		}
		// Otherwise, sort by name
		return node.Children[i].Name < node.Children[j].Name
	})

	// Recursively sort each child's children
	for i := range node.Children {
		if !node.Children[i].IsLeaf {
			sortTreeNodes(&node.Children[i])
		}
	}
}

func GetNodeForPath(root *types.TreeNode, path string) (*types.TreeNode, error) {
	// Handle empty path or root path
	if path == "" || path == "/" {
		return root, nil
	}

	// Normalize path: ensure it starts with / and doesn't end with /
	normalizedPath := path
	if !strings.HasPrefix(normalizedPath, "/") {
		normalizedPath = "/" + normalizedPath
	}
	if strings.HasSuffix(normalizedPath, "/") && normalizedPath != "/" {
		normalizedPath = normalizedPath[:len(normalizedPath)-1]
	}

	// Split the path into components
	components := strings.Split(strings.TrimPrefix(normalizedPath, "/"), "/")

	// Start searching from the root
	currentNode := root

	// Traverse the tree according to the path components
	for _, component := range components {
		if component == "" {
			continue
		}

		// Look for a child with matching name
		found := false
		for i := range currentNode.Children {
			if currentNode.Children[i].Name == component {
				currentNode = &currentNode.Children[i]
				found = true
				break
			}
		}

		// If no matching child was found, return an error
		if !found {
			return nil, fmt.Errorf("path not found: %s (component '%s' not found)", path, component)
		}
	}

	return currentNode, nil
}

func GetAllFiles(root *types.TreeNode) ([]*types.File, error) {
	// Check if root is nil
	if root == nil {
		return nil, fmt.Errorf("root node is nil")
	}

	// Initialize the files slice
	var files []*types.File

	// Call helper function to recursively collect files
	collectFiles(root, &files)

	return files, nil
}

// Helper function to recursively collect files from the tree
func collectFiles(node *types.TreeNode, files *[]*types.File) {
	// Process current node
	if node.IsLeaf && node.File != nil {
		// This is a file node, add its file to the collection
		*files = append(*files, node.File)
	}

	// Recursively process all children
	for i := range node.Children {
		collectFiles(&node.Children[i], files)
	}
}
