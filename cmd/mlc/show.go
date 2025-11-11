package main

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/franz/music-janitor/internal/store"
	"github.com/franz/music-janitor/internal/util"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var showCmd = &cobra.Command{
	Use:   "show",
	Short: "Show the current plan and file mappings",
	Long: `Display the execution plan in a human-readable format.

Shows what actions will be taken for each file:
- Source path → Destination path
- Action (copy/move/skip)
- Reason (winner, duplicate, score)
- Quality scores for duplicates

Use this to review the plan before executing.`,
	RunE: runShow,
}

func init() {
	rootCmd.AddCommand(showCmd)

	// Show-specific flags
	showCmd.Flags().Bool("duplicates-only", false, "Show only duplicate clusters")
	showCmd.Flags().Bool("winners-only", false, "Show only files that will be copied/moved")
	showCmd.Flags().Bool("verbose", false, "Show detailed metadata and scores")
	showCmd.Flags().Bool("tree", false, "Show destination folder structure as a tree")
	showCmd.Flags().IntP("depth", "L", 0, "Limit tree depth (0 = unlimited, only with --tree)")
	showCmd.Flags().Bool("dirs-only", false, "Show only directories in tree (only with --tree)")
}

func runShow(cmd *cobra.Command, args []string) error {
	dbPath := viper.GetString("db")
	duplicatesOnly, _ := cmd.Flags().GetBool("duplicates-only")
	winnersOnly, _ := cmd.Flags().GetBool("winners-only")
	verbose, _ := cmd.Flags().GetBool("verbose")
	showTree, _ := cmd.Flags().GetBool("tree")
	treeDepth, _ := cmd.Flags().GetInt("depth")
	dirsOnly, _ := cmd.Flags().GetBool("dirs-only")

	// Open database
	db, err := store.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Check if we have plans
	allPlans, err := db.GetAllPlans()
	if err != nil {
		return fmt.Errorf("failed to get plans: %w", err)
	}

	if len(allPlans) == 0 {
		util.WarnLog("No plans found. Run 'mlc plan' first.")
		return nil
	}

	// If tree mode, show tree and exit
	if showTree {
		return showDestinationTree(db, treeDepth, dirsOnly)
	}

	// Get all clusters
	clusters, err := db.GetAllClusters()
	if err != nil {
		return fmt.Errorf("failed to get clusters: %w", err)
	}

	util.InfoLog("=== Execution Plan ===")
	util.InfoLog("Database: %s", dbPath)
	util.InfoLog("")

	// Summary stats
	copyPlans, _ := db.CountPlansByAction("copy")
	movePlans, _ := db.CountPlansByAction("move")
	hardlinkPlans, _ := db.CountPlansByAction("hardlink")
	symlinkPlans, _ := db.CountPlansByAction("symlink")
	skipPlans, _ := db.CountPlansByAction("skip")

	util.InfoLog("Summary:")
	if copyPlans > 0 {
		util.InfoLog("  Copy: %d files", copyPlans)
	}
	if movePlans > 0 {
		util.InfoLog("  Move: %d files", movePlans)
	}
	if hardlinkPlans > 0 {
		util.InfoLog("  Hardlink: %d files", hardlinkPlans)
	}
	if symlinkPlans > 0 {
		util.InfoLog("  Symlink: %d files", symlinkPlans)
	}
	if skipPlans > 0 {
		util.InfoLog("  Skip (duplicates): %d files", skipPlans)
	}
	util.InfoLog("")

	// Show clusters
	duplicateCount := 0
	singletonCount := 0

	for _, cluster := range clusters {
		members, err := db.GetClusterMembers(cluster.ClusterKey)
		if err != nil {
			util.ErrorLog("Failed to get members for cluster %s: %v", cluster.ClusterKey, err)
			continue
		}

		isDuplicate := len(members) > 1

		// Filter based on flags
		if duplicatesOnly && !isDuplicate {
			continue
		}
		if winnersOnly && isDuplicate {
			// Only show the winner in duplicate clusters
			var winner *store.ClusterMember
			for _, m := range members {
				if m.Preferred {
					winner = m
					break
				}
			}
			if winner != nil {
				members = []*store.ClusterMember{winner}
			}
		}

		if isDuplicate {
			duplicateCount++
			// Show duplicate cluster header
			fmt.Println()
			util.WarnLog("Duplicate Cluster: %s", cluster.Hint)
			util.InfoLog("Cluster Key: %s", cluster.ClusterKey)
			util.InfoLog("Files: %d", len(members))
			fmt.Println()
		} else {
			singletonCount++
			if !duplicatesOnly {
				fmt.Println()
			}
		}

		// Show each member
		for i, member := range members {
			file, err := db.GetFileByID(member.FileID)
			if err != nil {
				util.ErrorLog("Failed to get file %d: %v", member.FileID, err)
				continue
			}

			plan, err := db.GetPlan(member.FileID)
			if err != nil || plan == nil {
				util.ErrorLog("Failed to get plan for file %d: %v", member.FileID, err)
				continue
			}

			metadata, _ := db.GetMetadata(member.FileID)

			// Show file info
			if isDuplicate {
				if member.Preferred {
					fmt.Print("  ✓ [WINNER] ")
				} else {
					fmt.Print("  ✗ [SKIP]   ")
				}
			} else {
				fmt.Print("  → ")
			}

			// Source path
			srcShort := filepath.Base(file.SrcPath)
			fmt.Printf("%s\n", srcShort)

			// Source full path
			fmt.Printf("     Source: %s\n", file.SrcPath)

			// Destination
			if plan.Action == "skip" {
				fmt.Printf("     Action: SKIP - %s\n", plan.Reason)
			} else {
				fmt.Printf("     Dest:   %s\n", plan.DestPath)
				fmt.Printf("     Action: %s\n", plan.Action)
			}

			// Quality score for duplicates
			if isDuplicate || verbose {
				fmt.Printf("     Score:  %.1f", member.QualityScore)
				if metadata != nil {
					fmt.Printf(" (%s", metadata.Codec)
					if metadata.Lossless {
						fmt.Print(", lossless")
					}
					if metadata.BitrateKbps > 0 {
						fmt.Printf(", %dkbps", metadata.BitrateKbps)
					}
					if metadata.BitDepth > 0 {
						fmt.Printf(", %d-bit", metadata.BitDepth)
					}
					if metadata.SampleRate > 0 {
						fmt.Printf(", %dHz", metadata.SampleRate)
					}
					fmt.Print(")")
				}
				fmt.Println()
			}

			// Verbose metadata
			if verbose && metadata != nil {
				if metadata.TagArtist != "" {
					fmt.Printf("     Artist: %s\n", metadata.TagArtist)
				}
				if metadata.TagAlbum != "" {
					fmt.Printf("     Album:  %s\n", metadata.TagAlbum)
				}
				if metadata.TagTitle != "" {
					fmt.Printf("     Title:  %s\n", metadata.TagTitle)
				}
				if metadata.DurationMs > 0 {
					fmt.Printf("     Length: %d:%02d\n", metadata.DurationMs/60000, (metadata.DurationMs/1000)%60)
				}
			}

			if isDuplicate && i < len(members)-1 {
				fmt.Println()
			}
		}
	}

	// Final summary
	fmt.Println()
	util.InfoLog("=== Statistics ===")
	util.InfoLog("Total clusters: %d", len(clusters))
	util.InfoLog("  Singletons: %d", singletonCount)
	util.InfoLog("  Duplicates: %d", duplicateCount)
	fmt.Println()

	if !winnersOnly {
		util.InfoLog("To see only files that will be copied/moved: mlc show --winners-only")
	}
	if !duplicatesOnly {
		util.InfoLog("To see only duplicate clusters: mlc show --duplicates-only")
	}
	util.InfoLog("To execute the plan: mlc execute")

	return nil
}

// showDestinationTree shows the planned destination folder structure as a tree
func showDestinationTree(db *store.Store, maxDepth int, dirsOnly bool) error {
	// Get all plans that will result in files at destination
	copyPlans, err := db.GetPlansByAction("copy")
	if err != nil {
		return fmt.Errorf("failed to get copy plans: %w", err)
	}

	movePlans, err := db.GetPlansByAction("move")
	if err != nil {
		return fmt.Errorf("failed to get move plans: %w", err)
	}

	hardlinkPlans, err := db.GetPlansByAction("hardlink")
	if err != nil {
		return fmt.Errorf("failed to get hardlink plans: %w", err)
	}

	symlinkPlans, err := db.GetPlansByAction("symlink")
	if err != nil {
		return fmt.Errorf("failed to get symlink plans: %w", err)
	}

	// Combine all plans
	allPlans := append(copyPlans, movePlans...)
	allPlans = append(allPlans, hardlinkPlans...)
	allPlans = append(allPlans, symlinkPlans...)

	if len(allPlans) == 0 {
		util.WarnLog("No files planned for destination")
		return nil
	}

	// Build and display tree
	tree := buildTree(allPlans, maxDepth, dirsOnly)
	output := generateTreeOutput(tree, dirsOnly)
	fmt.Print(output)

	return nil
}

// TreeNode represents a node in the folder tree
type TreeNode struct {
	Name      string
	IsDir     bool
	Children  map[string]*TreeNode
	FileCount int // For directories (when using depth limit or dirs-only)
}

// buildTree builds a tree structure from plans
func buildTree(plans []*store.Plan, maxDepth int, dirsOnly bool) *TreeNode {
	root := &TreeNode{
		Name:     ".",
		IsDir:    true,
		Children: make(map[string]*TreeNode),
	}

	for _, plan := range plans {
		if plan.DestPath == "" {
			continue
		}

		parts := strings.Split(filepath.Clean(plan.DestPath), string(filepath.Separator))

		// Apply depth limit if specified
		if maxDepth > 0 && len(parts) > maxDepth {
			parts = parts[:maxDepth]
		}

		current := root
		for i, part := range parts {
			if part == "" || part == "." {
				continue
			}

			isLastPart := i == len(parts)-1
			isFile := isLastPart && !strings.HasSuffix(part, "/")

			// Skip files if dirs-only mode
			if dirsOnly && isFile {
				current.FileCount++
				continue
			}

			if _, exists := current.Children[part]; !exists {
				current.Children[part] = &TreeNode{
					Name:     part,
					IsDir:    !isFile,
					Children: make(map[string]*TreeNode),
				}
			}

			if !isFile {
				current = current.Children[part]
			}
		}
	}

	return root
}

// generateTreeOutput generates a tree-like string representation
func generateTreeOutput(node *TreeNode, dirsOnly bool) string {
	var sb strings.Builder

	// Header
	sb.WriteString(".\n")

	// Generate tree lines
	generateTreeLines(node, "", true, &sb, dirsOnly)

	// Footer with statistics
	stats := calculateStats(node)
	sb.WriteString(fmt.Sprintf("\n%d directories", stats.dirs))
	if !dirsOnly {
		sb.WriteString(fmt.Sprintf(", %d files", stats.files))
	}
	sb.WriteString("\n")

	return sb.String()
}

// generateTreeLines recursively generates tree lines
func generateTreeLines(node *TreeNode, prefix string, isLast bool, sb *strings.Builder, dirsOnly bool) {
	if node.Name == "." {
		// Root node - process children
		sortedChildren := getSortedChildren(node)
		for i, child := range sortedChildren {
			isLastChild := i == len(sortedChildren)-1
			generateTreeLines(child, "", isLastChild, sb, dirsOnly)
		}
		return
	}

	// Determine tree characters
	var connector, extension string
	if isLast {
		connector = "└── "
		extension = "    "
	} else {
		connector = "├── "
		extension = "│   "
	}

	// Write current node
	displayName := node.Name
	if node.IsDir {
		displayName += "/"
	}
	if dirsOnly && node.FileCount > 0 {
		displayName += fmt.Sprintf(" (%d files)", node.FileCount)
	}

	sb.WriteString(prefix + connector + displayName + "\n")

	// Recursively process children
	if node.IsDir && len(node.Children) > 0 {
		sortedChildren := getSortedChildren(node)
		newPrefix := prefix + extension

		for i, child := range sortedChildren {
			isLastChild := i == len(sortedChildren)-1
			generateTreeLines(child, newPrefix, isLastChild, sb, dirsOnly)
		}
	}
}

// getSortedChildren returns children sorted (dirs first, then alphabetically)
func getSortedChildren(node *TreeNode) []*TreeNode {
	children := make([]*TreeNode, 0, len(node.Children))
	for _, child := range node.Children {
		children = append(children, child)
	}

	sort.Slice(children, func(i, j int) bool {
		// Directories before files
		if children[i].IsDir != children[j].IsDir {
			return children[i].IsDir
		}
		// Alphabetical within same type
		return strings.ToLower(children[i].Name) < strings.ToLower(children[j].Name)
	})

	return children
}

// Stats holds tree statistics
type Stats struct {
	dirs  int
	files int
}

// calculateStats recursively calculates tree statistics
func calculateStats(node *TreeNode) Stats {
	stats := Stats{}

	if node.IsDir {
		stats.dirs++
		for _, child := range node.Children {
			childStats := calculateStats(child)
			stats.dirs += childStats.dirs
			stats.files += childStats.files
		}
		stats.files += node.FileCount // Add uncounted files from depth limit
	} else {
		stats.files++
	}

	return stats
}
