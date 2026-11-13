package httpapi

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/tabunugoku/mars/server/internal/natsort"
)

// maxDirectoryEntries caps one listing so a folder with thousands of
// children cannot make the response large.
const maxDirectoryEntries = 2000

// ListDirectories lists the folders directly inside a server folder, for
// picking a library folder. Administrators only (see access.go).
func (s *server) ListDirectories(ctx context.Context, req ListDirectoriesRequestObject) (ListDirectoriesResponseObject, error) {
	path := ""
	if req.Params.Path != nil {
		path = *req.Params.Path
	}
	if path == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		path = filepath.VolumeName(wd) + string(filepath.Separator)
	}
	if !filepath.IsAbs(path) {
		return ListDirectories400JSONResponse{badRequest(ctx, "bad_request.root_not_absolute")}, nil
	}
	path = filepath.Clean(path)

	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return ListDirectories404JSONResponse{notFound(ctx, "not_found.folder")}, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return ListDirectories404JSONResponse{notFound(ctx, "not_found.folder")}, nil
	}

	entries, err := os.ReadDir(path)
	if errors.Is(err, fs.ErrPermission) {
		return ListDirectories400JSONResponse{badRequest(ctx, "bad_request.folder_unreadable")}, nil
	}
	if err != nil {
		return nil, err
	}

	var names []string
	for _, e := range entries {
		name := e.Name()
		if name[0] == '.' {
			continue
		}
		isDir := e.IsDir()
		if !isDir && e.Type()&fs.ModeSymlink != 0 {
			if st, err := os.Stat(filepath.Join(path, name)); err == nil && st.IsDir() {
				isDir = true
			}
		}
		if isDir {
			names = append(names, name)
		}
	}
	sort.Slice(names, func(i, j int) bool { return natsort.Less(names[i], names[j]) })
	if len(names) > maxDirectoryEntries {
		names = names[:maxDirectoryEntries]
	}

	dirs := make([]DirectoryEntry, 0, len(names))
	for _, name := range names {
		child := filepath.Join(path, name)
		dirs = append(dirs, DirectoryEntry{
			Name:     name,
			Path:     child,
			Readable: canOpenDir(child),
		})
	}

	out := DirectoryListing{Path: path, Directories: dirs}
	if parent := filepath.Dir(path); parent != path {
		out.Parent = &parent
	}
	return ListDirectories200JSONResponse(out), nil
}

// canOpenDir reports whether the directory can be opened for listing.
func canOpenDir(p string) bool {
	f, err := os.Open(p)
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}
