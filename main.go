package main

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"syscall"
	"time"

	tcell "github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// Global variables to track visual selection in the file table.
var (
	visualMode                   bool = false
	selectionStart, selectionEnd int  = -1, -1
)

// DirTree holds information about a directory, including its aggregated size,
// its subdirectories, and the files directly in it.
type DirTree struct {
	Name    string
	Path    string
	Size    int64         // aggregated size in bytes (files + subdirectories)
	SubDirs []*DirTree    // subdirectories
	Files   []os.FileInfo // files directly in this directory
}

// buildDirTree recursively scans the directory at the given path and builds a DirTree.
func buildDirTree(path string, mmin int) (*DirTree, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	tree := &DirTree{
		Name: info.Name(),
		Path: path,
		Size: 0,
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return tree, nil
	}

	now := time.Now()

	for _, entry := range entries {
		fullPath := filepath.Join(path, entry.Name())
		if entry.IsDir() {
			subTree, err := buildDirTree(fullPath, mmin)
			if err == nil {
				tree.SubDirs = append(tree.SubDirs, subTree)
				tree.Size += subTree.Size
			}
		} else {
			fileInfo, err := entry.Info()
			if err != nil {
				continue
			}
			// If mmin is greater than 0 then filter out files older than mmin minutes.
			if mmin > 0 {
				minutesOld := now.Sub(fileInfo.ModTime()).Minutes()
				if minutesOld >= float64(mmin) {
					continue
				}
			}
			tree.Files = append(tree.Files, fileInfo)
			tree.Size += fileInfo.Size()
		}
	}

	return tree, nil
}

// humanizeBytes converts a number of bytes into a human-readable string.
func humanizeBytes(s int64) string {
	const unit = 1024
	if s < unit {
		return fmt.Sprintf("%d B", s)
	}
	div, exp := int64(unit), 0
	for n := s / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(s)/float64(div), "KMGTPE"[exp])
}

// fileDetails returns a slice of strings with ls -al style details for a file.
func fileDetails(info os.FileInfo) []string {
	// Permissions.
	perms := info.Mode().String()

	// Try to get underlying Stat_t for extra details.
	var links uint64 = 0
	uid := ""
	gid := ""
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		links = uint64(stat.Nlink)
		uid = strconv.Itoa(int(stat.Uid))
		gid = strconv.Itoa(int(stat.Gid))

		// Lookup owner name.
		if u, err := user.LookupId(uid); err == nil {
			uid = u.Username
		}
		// Lookup group name.
		if g, err := user.LookupGroupId(gid); err == nil {
			gid = g.Name
		}
	}

	// File size.
	size := humanizeBytes(info.Size())

	// Modified time (format similar to ls).
	modTime := info.ModTime().Format("Jan 02 15:04")

	// File name (append "/" if directory).
	name := info.Name()
	if info.IsDir() {
		name += "/"
	}

	return []string{perms, fmt.Sprintf("%d", links), uid, gid, size, modTime, name}
}

// updateFileTable updates the provided tview.Table with file details for the given directory tree.
func updateFileTable(table *tview.Table, dt *DirTree) {
	table.Clear()

	// Set table headers.
	headers := []string{"Permissions", "Links", "Owner", "Group", "Size", "Modified", "Name"}
	for i, h := range headers {
		cell := tview.NewTableCell("[::b]" + h).
			SetTextColor(tcell.ColorYellow).
			SetAlign(tview.AlignLeft)
		table.SetCell(0, i, cell)
	}

	// Sort files by size descending.
	files := dt.Files
	sort.Slice(files, func(i, j int) bool {
		return files[i].Size() > files[j].Size()
	})

	// Add file rows.
	for r, file := range files {
		details := fileDetails(file)
		for c, d := range details {
			cell := tview.NewTableCell(d).
				SetAlign(tview.AlignLeft)
			table.SetCell(r+1, c, cell)
		}
	}
}

func main() {
	// Allow passing the base directory as a command-line argument.
	rootPath := "."
	mmin := 0 // 0 means no filtering

	if len(os.Args) > 1 {
		rootPath = os.Args[1]
		// Simple tilde expansion if the path starts with '~'
		if rootPath[0] == '~' {
			if usr, err := user.Current(); err == nil {
				rootPath = filepath.Join(usr.HomeDir, rootPath[1:])
			}
		}
	}

	// Optionally, a second argument: mmin (in minutes)
	if len(os.Args) > 2 {
		if val, err := strconv.Atoi(os.Args[2]); err == nil {
			mmin = val
		}
	}

	app := tview.NewApplication()
	scanApp := tview.NewApplication()

	// Create a TextView to display the scanning progress.
	scanTextView := tview.NewTextView()
	scanTextView.SetBorder(true)
	scanTextView.SetTitle("Scanning")
	scanTextView.SetTextAlign(tview.AlignCenter)
	scanTextView.SetText(fmt.Sprintf("Scanning folder:\n%s\nPlease wait...", rootPath))

	// Set up a spinner to show progress.
	spinnerChars := []rune{'|', '/', '-', '\\'}
	spinnerIndex := 0
	ticker := time.NewTicker(200 * time.Millisecond)
	spinnerDone := make(chan struct{})
	go func() {
		for {
			select {
			case <-ticker.C:
				app.QueueUpdateDraw(func() {
					scanTextView.SetText(fmt.Sprintf("Scanning folder:\n%s\n%s", rootPath, string(spinnerChars[spinnerIndex])))
				})
				spinnerIndex = (spinnerIndex + 1) % len(spinnerChars)
			case <-spinnerDone:
				return
			}
		}
	}()

	// Start scanning in a separate goroutine.
	var mainFlex *tview.Flex
	var rootTree *DirTree
	var scanErr error
	go func() {
		rootTree, scanErr = buildDirTree(rootPath, mmin)
		scanApp.Stop()
		ticker.Stop()
		close(spinnerDone)
	}()

	// Set the scanning view as the initial root.
	if err := scanApp.SetRoot(scanTextView, true).SetFocus(scanTextView).Run(); err != nil {
		panic(err)
	}

	if scanErr != nil {
		app.Stop()
		fmt.Fprintf(os.Stderr, "Error scanning directory: %v\n", scanErr)
		os.Exit(1)
	}

	// --- Build the main UI (tree view and file table) using rootTree ---

	// Create the tree root node.
	treeRoot := tview.NewTreeNode(fmt.Sprintf("%s (%s)", rootTree.Name, humanizeBytes(rootTree.Size))).
		SetReference(rootTree).
		SetExpanded(len(rootTree.SubDirs) > 0)

	// Create the left pane: a tree view.
	treeView := tview.NewTreeView().
		SetRoot(treeRoot).
		SetCurrentNode(treeRoot)
	treeView.SetBorder(true)
	treeView.SetBorderColor(tcell.ColorGreen)
	treeView.SetTitle("Directories")

	// Recursive function to add child nodes.
	addTreeNodes := func(tn *tview.TreeNode, dt *DirTree) {
		// Sort subdirectories by size (largest first).
		sort.Slice(dt.SubDirs, func(i, j int) bool {
			return dt.SubDirs[i].Size > dt.SubDirs[j].Size
		})
		for _, sub := range dt.SubDirs {
			nodeText := fmt.Sprintf("%s (%s)", sub.Name, humanizeBytes(sub.Size))
			child := tview.NewTreeNode(nodeText).
				SetReference(sub)
			if len(sub.SubDirs) > 0 {
				child.SetExpanded(true)
			}
			tn.AddChild(child)
		}
	}
	// Prepopulate the first level.
	addTreeNodes(treeRoot, rootTree)

	// Create the right pane: a table to show file details.
	fileTable := tview.NewTable()
	fileTable.SetFixed(1, 0)
	fileTable.SetBorders(false)
	fileTable.SetBorder(true)
	fileTable.SetTitle("Files")
	fileTable.SetSelectable(true, false)

	fileTable.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		// Ensure we have the current directory from the tree view.
		node := treeView.GetCurrentNode()
		dt, ok := node.GetReference().(*DirTree)
		if !ok {
			return event
		}

		switch event.Key() {
		case tcell.KeyRune:
			switch event.Rune() {
			case 'h': // Switch focus to the directories pane.
				app.SetFocus(treeView)
				treeView.SetBorderColor(tcell.ColorGreen)
				fileTable.SetBorderColor(tview.Styles.BorderColor)
				return nil
			case 'v':
				// Toggle visual (multi‑selection) mode.
				if !visualMode {
					// Enter visual mode.
					visualMode = true
					// Record the starting row.
					row, _ := fileTable.GetSelection()
					selectionStart = row
					selectionEnd = row
					highlightVisualSelection(fileTable, selectionStart, selectionEnd)
				} else {
					// Exit visual mode.
					visualMode = false
					// Optionally, leave the selection intact for deletion
					// or clear it. Here we "revert" to normal highlighting:
					clearVisualSelection(fileTable)
				}
				return nil
			case 'j':
				// Move down.
				row, _ := fileTable.GetSelection()
				if row < fileTable.GetRowCount()-1 {
					fileTable.Select(row+1, 0)
					if visualMode {
						selectionEnd = row + 1
						highlightVisualSelection(fileTable, selectionStart, selectionEnd)
					}
				}
				return nil
			case 'k':
				// Move up.
				row, _ := fileTable.GetSelection()
				if row > 1 { // row 0 is header.
					fileTable.Select(row-1, 0)
					if visualMode {
						selectionEnd = row - 1
						highlightVisualSelection(fileTable, selectionStart, selectionEnd)
					}
				}
				return nil
			case 'd':
				// Handle deletion.
				var selectedRows []int
				if visualMode {
					// Use the range between selectionStart and selectionEnd.
					start, end := selectionStart, selectionEnd
					if start > end {
						start, end = end, start
					}
					for r := start; r <= end; r++ {
						selectedRows = append(selectedRows, r)
					}
				} else {
					// No visual mode—just the current row.
					row, _ := fileTable.GetSelection()
					selectedRows = []int{row}
				}
				// Build the list of file names.
				var filesToDelete []string
				for _, r := range selectedRows {
					if r > 0 && r-1 < len(dt.Files) { // row 0 is header.
						fileInfo := dt.Files[r-1]
						filesToDelete = append(filesToDelete, fileInfo.Name())
					}
				}
				if len(filesToDelete) == 0 {
					return event
				}
				// Show the deletion confirmation modal.
				showMultiDeleteModal(app, dt.Path, filesToDelete, func(deleted bool) {
					if deleted {
						// Remove deleted files from dt.Files.
						var newFiles []os.FileInfo
						for _, fileInfo := range dt.Files {
							keep := true
							for _, name := range filesToDelete {
								if fileInfo.Name() == name {
									keep = false
									break
								}
							}
							if keep {
								newFiles = append(newFiles, fileInfo)
							}
						}
						dt.Files = newFiles
						updateFileTable(fileTable, dt)
					} else {
						clearVisualSelection(fileTable)
					}
					// Reset selection state.
					visualMode = false
					selectionStart = -1
					selectionEnd = -1
					app.SetRoot(mainFlex, true)
					app.SetFocus(fileTable)
				})
				return nil
			}
		}
		return event
	})

	updateFileTable(fileTable, rootTree)

	// When the selection changes in the tree, update the file table.
	treeView.SetChangedFunc(func(node *tview.TreeNode) {
		if dt, ok := node.GetReference().(*DirTree); ok {
			updateFileTable(fileTable, dt)
		}
	})
	// Toggle expansion when a node is selected.
	treeView.SetSelectedFunc(func(node *tview.TreeNode) {
		if node.IsExpanded() {
			node.CollapseAll()
		} else {
			node.Expand()
			if len(node.GetChildren()) == 0 {
				if dt, ok := node.GetReference().(*DirTree); ok {
					addTreeNodes(node, dt)
				}
			}
		}
	})

	treeView.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyRune {
			switch event.Rune() {
			case 'l':
				// Switch focus to the files pane.
				app.SetFocus(fileTable)
				fileTable.SetBorderColor(tcell.ColorGreen)
				treeView.SetBorderColor(tview.Styles.BorderColor)
				return nil
			case 'r':
				// Refresh the selected directory node.
				selectedNode := treeView.GetCurrentNode()
				dt, ok := selectedNode.GetReference().(*DirTree)
				if !ok {
					return event
				}
				// Re-scan the directory for updated disk usage.
				newDt, err := buildDirTree(dt.Path, mmin)
				if err != nil {
					// (Optional) You might display an error message here.
					return event
				}
				// Update the node's reference and text with the new size.
				selectedNode.SetReference(newDt)
				selectedNode.SetText(fmt.Sprintf("%s (%s)", newDt.Name, humanizeBytes(newDt.Size)))
				// If the node is expanded, clear and repopulate its children.
				if selectedNode.IsExpanded() {
					selectedNode.ClearChildren()
					addTreeNodes(selectedNode, newDt)
				}
				// Also update the file table if the refreshed node is selected.
				updateFileTable(fileTable, newDt)
				return nil
			}
		}
		return event
	})

	thinGreenStyle := tcell.StyleDefault.
		Foreground(tcell.ColorGreen).
		Background(tcell.ColorBlack)

	// mainFlex holds our two panes.
	mainFlex = tview.NewFlex()
	mainFlex.AddItem(treeView, 0, 1, true).SetBorderStyle(thinGreenStyle)
	mainFlex.AddItem(fileTable, 0, 2, false) // right pane

	app.SetBeforeDrawFunc(func(screen tcell.Screen) bool {
		if visualMode {
			// Get the current selected row.
			row, _ := fileTable.GetSelection()
			// Override the default selection style by setting the cell backgrounds to blue.
			for col := 0; col < fileTable.GetColumnCount(); col++ {
				cell := fileTable.GetCell(row, col)
				cell.SetBackgroundColor(tcell.ColorRed)
				cell.SetTextColor(tcell.ColorBlack)
			}
		}
		return false
	})

	// Global key handler to capture "q" for quit and "?" for help.
	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		// Quit confirmation.
		if event.Key() == tcell.KeyRune && event.Rune() == 'q' {
			showQuitModal(app, mainFlex)
			return nil
		}
		// Help dialog.
		if event.Key() == tcell.KeyRune && event.Rune() == '?' {
			showHelpModal(app, mainFlex)
			return nil
		}
		return event
	})

	// Finally, set the main UI as the new root and start the application.
	if err := app.SetRoot(mainFlex, true).SetFocus(treeView).Run(); err != nil {
		panic(err)
	}
}

// showQuitModal displays a confirmation dialog for quitting.
func showQuitModal(app *tview.Application, mainFlex tview.Primitive) {
	modal := tview.NewModal()
	modal.SetBackgroundColor(tcell.ColorBlack)
	modal.SetText("[white]Do you really want to quit?").
		AddButtons([]string{"Yes", "No"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			if buttonLabel == "Yes" {
				app.Stop()
			} else {
				app.SetRoot(mainFlex, true)
			}
		})
	modal.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEnter:
			app.Stop()
			return nil
		case tcell.KeyEscape:
			app.SetRoot(mainFlex, true)
			return nil
		}
		if event.Key() == tcell.KeyRune {
			switch event.Rune() {
			case 'y', 'Y':
				app.Stop()
				return nil
			case 'n', 'N':
				app.SetRoot(mainFlex, true)
				return nil
			}
		}
		return event
	})
	app.SetRoot(modal, true)
}

// showHelpModal displays a help dialog listing key commands.
func showHelpModal(app *tview.Application, mainFlex tview.Primitive) {
	helpText := `
  Key Bindings:
  l : Focus Files Pane
  h : Focus Directories Pane
  j : Move down the file list
  k : Move up the file list
  d : Delete the selected file
  v : Select multiple files / disable multiple selection
  r : Refresh the selected directory disk usage
  q : Quit (with confirmation)
  ? : Show this help dialog
  
  Use arrow keys or j/k to navigate.
`
	// Create a TextView with left-aligned text.
	textView := tview.NewTextView()
	textView.SetTextAlign(tview.AlignLeft)
	textView.SetTextColor(tcell.ColorWhite)
	textView.SetBackgroundColor(tcell.ColorBlack)
	textView.SetBorder(true)
	textView.SetTitle("Help")
	textView.SetText(helpText)

	// Center the TextView horizontally and vertically by nesting two Flex layouts.
	modal := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false). // Top spacer.
		AddItem(tview.NewFlex().
			AddItem(nil, 0, 1, false).      // Left spacer.
			AddItem(textView, 65, 1, true). // The help box (80 columns wide; adjust as needed).
			AddItem(nil, 0, 1, false),      // Right spacer.
						0, 2, true).
		AddItem(nil, 0, 1, false) // Bottom spacer.

	app.SetRoot(modal, true).SetFocus(textView)
	textView.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		// Dismiss the help modal on any key press.
		app.SetRoot(mainFlex, true)
		return nil
	})
}

func showMultiDeleteModal(app *tview.Application, basePath string, fileNames []string, callback func(deleted bool)) {
	text := fmt.Sprintf("Do you really want to delete %d files?\n", len(fileNames))
	for _, name := range fileNames {
		text += fmt.Sprintf(" - %s\n", name)
	}
	modal := tview.NewModal()
	modal.SetBackgroundColor(tcell.ColorBlack)
	modal.SetText("[white]" + text).
		AddButtons([]string{"Yes", "No"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			if buttonLabel == "Yes" {
				// Delete each file.
				for _, name := range fileNames {
					fullPath := filepath.Join(basePath, name)
					os.Remove(fullPath)
				}
				callback(true)
			} else {
				callback(false)
			}
		})
	modal.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyRune {
			switch event.Rune() {
			case 'y', 'Y':
				for _, name := range fileNames {
					fullPath := filepath.Join(basePath, name)
					os.Remove(fullPath)
				}
				callback(true)
				return nil
			default:
				callback(false)
				return nil
			}
		}
		return event
	})
	app.SetRoot(modal, true)
}

// highlightVisualSelection updates the background color for rows in the selected range.
func highlightVisualSelection(table *tview.Table, start, end int) {
	table.SetSelectedStyle(tcell.StyleDefault.Background(tcell.ColorRed).Foreground(tcell.ColorBlack))
	if start > end {
		start, end = end, start
	}
	// Loop over all file rows (skip header row at index 0).
	for row := 1; row <= table.GetRowCount(); row++ {
		for col := 0; col < table.GetColumnCount(); col++ {
			cell := table.GetCell(row, col)
			if row >= start && row <= end {
				cell.SetBackgroundColor(tcell.ColorRed)
				cell.SetTextColor(tcell.ColorBlack)
				cell.SetAttributes(cell.Attributes | tcell.AttrStrikeThrough)
			} else {
				cell.SetBackgroundColor(tview.Styles.PrimitiveBackgroundColor)
				cell.SetTextColor(tview.Styles.PrimaryTextColor)
			}
		}
	}
}

// clearVisualSelection resets the background color for all file rows.
func clearVisualSelection(table *tview.Table) {
	table.SetSelectedStyle(tcell.StyleDefault.Background(tview.Styles.PrimaryTextColor).Foreground(tview.Styles.PrimitiveBackgroundColor))
	for row := 1; row < table.GetRowCount(); row++ {
		for col := 0; col < table.GetColumnCount(); col++ {
			// table.GetCell(row, col).SetBackgroundColor(tcell.ColorDefault)
			cell := table.GetCell(row, col)
			cell.SetBackgroundColor(tview.Styles.PrimitiveBackgroundColor)
			cell.SetTextColor(tview.Styles.PrimaryTextColor)
			cell.SetAttributes(cell.Attributes &^ tcell.AttrStrikeThrough)
		}
	}
}
