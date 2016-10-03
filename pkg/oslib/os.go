package oslib

import (
	"os"
	"strconv"
	"strings"
)

var (
	quotedPathSep = strconv.QuoteRuneToASCII(os.PathSeparator)
	PathSep       = quotedPathSep[1 : len(quotedPathSep)-1]
)

// PathExists returns whether or not the referenced filesystem path exists.
func PathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// PathBaseName takes a full path and returns the end-most path name - this is the base name!
// Counter-part of PathDirName.
func PathBaseName(path string) string {
	pieces := strings.Split(path, "/")
	baseName := pieces[len(pieces)-1]
	return baseName
}

// PathDirName takes a full path and returns the parent-directory portion of it.
// Counter-part of PathBaseName.
func PathDirName(path string) string {
	pieces := strings.Split(path, "/")
	dirName := strings.Join(pieces[0:len(pieces)-1], "/")
	return dirName
}

// IsDirectory returns a boolean indicating if the provided path is a directory.
func IsDirectory(path string) (bool, error) {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	return fileInfo.IsDir(), nil
}

// IsRegularFile returns a boolean indicating if the provided path is a regular file.
func IsRegularFile(path string) (bool, error) {
	stat, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	return stat.Mode().IsRegular(), nil
}

// OsPath returns a fully assembled path string with appropriate OS-specific
// separators.
func OsPath(pathElements ...string) string {
	return strings.Join(pathElements, PathSep)
}
