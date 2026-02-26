package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/user"
	"strconv"
	"syscall"
	"time"
)

var (
	flagLong  bool
	flagAll   bool
	flagHuman bool
)

func init() {
	flag.BoolVar(&flagLong, "l", false, "long format with metadata")
	flag.BoolVar(&flagAll, "a", false, "include hidden files (starting with .)")
	flag.BoolVar(&flagHuman, "h", false, "human-readable sizes (use with -l)")
}

// FileEntry is the JSON shape for a file in long (-l) format.
type FileEntry struct {
	Name      string `json:"name"`
	Size      int64  `json:"size"`
	SizeHuman string `json:"size_human,omitempty"`
	Mode      string `json:"mode"`
	IsDir     bool   `json:"is_dir"`
	Modified  string `json:"modified"`
	Owner     string `json:"owner,omitempty"`
	Group     string `json:"group,omitempty"`
}

func humanSize(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%dB", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%c", float64(b)/float64(div), "KMGTPE"[exp])
}

func makeEntry(info fs.FileInfo) FileEntry {
	e := FileEntry{
		Name:     info.Name(),
		Size:     info.Size(),
		Mode:     info.Mode().String(),
		IsDir:    info.IsDir(),
		Modified: info.ModTime().Format(time.RFC3339),
	}
	if flagHuman {
		e.SizeHuman = humanSize(info.Size())
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		uid := strconv.Itoa(int(stat.Uid))
		gid := strconv.Itoa(int(stat.Gid))
		if u, err := user.LookupId(uid); err == nil {
			e.Owner = u.Username
		} else {
			e.Owner = uid
		}
		if g, err := user.LookupGroupId(gid); err == nil {
			e.Group = g.Name
		} else {
			e.Group = gid
		}
	}
	return e
}

func listDir(path string) (any, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}

	if flagLong {
		result := make([]FileEntry, 0, len(entries))
		for _, entry := range entries {
			if !flagAll && entry.Name()[0] == '.' {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				continue
			}
			result = append(result, makeEntry(info))
		}
		return result, nil
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !flagAll && entry.Name()[0] == '.' {
			continue
		}
		names = append(names, entry.Name())
	}
	return names, nil
}

// expandFlags splits combined short flags like -la into -l -a so that
// Go's flag package can parse them individually.
func expandFlags(args []string) []string {
	expanded := make([]string, 0, len(args))
	for _, arg := range args {
		if len(arg) > 2 && arg[0] == '-' && arg[1] != '-' {
			for _, c := range arg[1:] {
				expanded = append(expanded, "-"+string(c))
			}
		} else {
			expanded = append(expanded, arg)
		}
	}
	return expanded
}

func main() {
	os.Args = append(os.Args[:1], expandFlags(os.Args[1:])...)
	flag.Parse()

	paths := flag.Args()
	if len(paths) == 0 {
		paths = []string{"."}
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")

	if len(paths) == 1 {
		info, err := os.Stat(paths[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "lsj: %v\n", err)
			os.Exit(1)
		}

		var result any
		if info.IsDir() {
			result, err = listDir(paths[0])
			if err != nil {
				fmt.Fprintf(os.Stderr, "lsj: %v\n", err)
				os.Exit(1)
			}
		} else {
			if flagLong {
				result = makeEntry(info)
			} else {
				result = info.Name()
			}
		}
		enc.Encode(result)
		return
	}

	// Multiple paths: object keyed by path.
	out := make(map[string]any, len(paths))
	exitCode := 0
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "lsj: %v\n", err)
			exitCode = 1
			continue
		}
		if info.IsDir() {
			dirResult, err := listDir(path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "lsj: %v\n", err)
				exitCode = 1
				continue
			}
			out[path] = dirResult
		} else {
			if flagLong {
				out[path] = makeEntry(info)
			} else {
				out[path] = info.Name()
			}
		}
	}
	enc.Encode(out)
	os.Exit(exitCode)
}
