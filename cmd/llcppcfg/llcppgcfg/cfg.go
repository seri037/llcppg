package llcppgcfg

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"unicode"

	"github.com/goplus/llcppg/types"
)

type CfgMode int

const (
	NormalMode CfgMode = iota
	ExpandMode
	SortMode
)

type ObjFile struct {
	OFile string
	HFile string
	Deps  []string
}

func NewObjFile(oFile, hFile string) *ObjFile {
	return &ObjFile{
		OFile: oFile,
		HFile: hFile,
		Deps:  make([]string, 0),
	}
}

func NewObjFileString(str string, relPath string) *ObjFile {
	fields := strings.Split(str, ":")
	if len(fields) != 2 {
		return nil
	}
	objFile := &ObjFile{
		OFile: fields[0],
		HFile: fields[1],
		Deps:  make([]string, 0),
	}
	return objFile
}

func (o *ObjFile) String() string {
	return fmt.Sprintf("{OFile:%s, HFile:%s, Deps:%v}", o.OFile, o.HFile, o.Deps)
}

type CflagEntry struct {
	Include  string
	ObjFiles []*ObjFile
}

func (c *CflagEntry) String() string {
	return fmt.Sprintf("{Include:%s, ObjFiles:%v}", c.Include, c.ObjFiles)
}

type LLCppConfig types.Config

type NilError struct {
}

func (p *NilError) Error() string {
	return "nil error"
}

func NewNilError() *NilError {
	return &NilError{}
}

type EmptyStringError struct {
	name string
}

func (p *EmptyStringError) Error() string {
	return p.name + " can't be empty"
}

func NewEmptyStringError(name string) *EmptyStringError {
	return &EmptyStringError{name: name}
}

func CmdOutString(cmd *exec.Cmd, dir string) (string, error) {
	if cmd == nil {
		return "", NewNilError()
	}
	outBuf := bytes.NewBufferString("")
	cmd.Stdin = os.Stdin
	cmd.Stdout = outBuf
	cmd.Env = os.Environ()
	if len(dir) > 0 {
		cmd.Dir = dir
	}
	err := cmd.Run()
	if err != nil {
		return outBuf.String(), err
	}
	return outBuf.String(), nil
}

func ExecCommand(cmdStr string, args ...string) *exec.Cmd {
	cmdStr = strings.TrimSpace(cmdStr)
	return exec.Command(cmdStr, args...)
}

func ExpandString(str string, dir string) (expand string, org string) {
	org = str
	str = strings.ReplaceAll(str, "(", "{")
	str = strings.ReplaceAll(str, ")", "}")
	expandStr := os.Expand(str, func(s string) string {
		args := strings.Fields(s)
		if len(args) == 0 {
			return ""
		}
		outString, err := CmdOutString(ExecCommand(args[0], args[1:]...), dir)
		if err != nil {
			return ""
		}
		return outString
	})
	expand = strings.TrimSpace(expandStr)
	return expand, org
}

func doExpandCflags(str string, fn func(s string) bool) ([]string, string) {
	list := strings.Fields(str)
	contains := make(map[string]string, 0)
	for _, l := range list {
		trimStr := strings.TrimPrefix(l, "-I")
		trimStr += "/"
		err := filepath.WalkDir(trimStr, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			if !fn(d.Name()) {
				return nil
			}
			_, ok := contains[path]
			if !ok {
				relPath, errRel := filepath.Rel(trimStr, path)
				if errRel != nil {
					return errRel
				}
				contains[path] = relPath
			}
			return nil
		})
		if err != nil {
			log.Println(err)
		}
	}

	includes := make([]string, 0)
	includeMap := make(map[string]struct{})
	for path, relPath := range contains {
		includeDir, found := strings.CutSuffix(path, relPath)
		if found {
			includeMap[includeDir] = struct{}{}
		}
		includes = append(includes, relPath)
	}
	var flagsBuilder strings.Builder
	for include := range includeMap {
		if flagsBuilder.Len() > 0 {
			flagsBuilder.WriteRune(' ')
		}
		flagsBuilder.WriteString("-I" + include)
	}
	flags := flagsBuilder.String()
	return includes, flags
}

func ExpandName(name string, dir string, libsOrCflags string) (expand string, org string) {
	originString := fmt.Sprintf("$(pkg-config --%s %s)", libsOrCflags, name)
	return ExpandString(originString, dir)
}

func ExpandLibsName(name string, dir string) (expand string, org string) {
	return ExpandName(name, dir, "libs")
}

func ExpandCflags(originCFlags string) (includes []string, expand string, org string) {
	cflags, orgCflags := ExpandString(originCFlags, "")
	expandIncludes, expandCflags := doExpandCflags(cflags, func(s string) bool {
		ext := filepath.Ext(s)
		return ext == ".h" || ext == ".hpp"
	})
	if len(expandCflags) > 0 {
		cflags = expandCflags
	}
	return expandIncludes, cflags, orgCflags
}

func ExpandCFlagsName(name string) (includes []string, expand string, org string) {
	originCFlags := fmt.Sprintf("$(pkg-config --cflags %s)", name)
	return ExpandCflags(originCFlags)
}

func expandCFlagsAndLibs(name string, cfg *LLCppConfig, dir string) {
	cfg.CFlags, _ = ExpandName(name, dir, "cflags")
	cfg.Libs, _ = ExpandLibsName(name, dir)
}

func findDepSlice(lines []string) ([]string, string) {
	objFileString := ""
	iStart := 0
	numLines := len(lines)
	complete := false
	for i := 0; i < numLines && !complete; i++ {
		line := lines[i]
		if strings.ContainsRune(line, rune(':')) && !strings.HasSuffix(line, ":") {
			objFileString = line
			iStart = i + 1
			break
		}
		complete = true
		for j := i + 1; j < numLines; j++ {
			line2 := lines[j]
			if len(line2) > 0 {
				iStart = j + 1
				objFileString = line + line2
				break
			}
		}
	}
	if iStart < numLines {
		return lines[iStart:], objFileString
	}
	return []string{}, objFileString
}

func parseFileEntry(trimStr, path string, d fs.DirEntry, exts []string, excludeSubdirs []string) *ObjFile {
	if d.IsDir() || strings.HasPrefix(d.Name(), ".") {
		return nil
	}
	idx := len(exts)
	for i, ext := range exts {
		if strings.HasSuffix(d.Name(), ext) {
			idx = i
			break
		}
	}
	if idx == len(exts) {
		return nil
	}
	relPath, err := filepath.Rel(trimStr, path)
	if err != nil {
		return nil
	}
	index := strings.IndexRune(relPath, filepath.Separator)
	if index >= 0 {
		dir := relPath[:index]
		for _, subdir := range excludeSubdirs {
			if subdir == dir {
				return nil
			}
		}
	}
	clangCmd := ExecCommand("clang", "-I"+trimStr, "-MM", relPath)
	outString, err := CmdOutString(clangCmd, trimStr)
	if err != nil || outString == "" {
		objFile := NewObjFile(relPath, relPath)
		return objFile
	}
	outString = strings.ReplaceAll(outString, "\\\n", "\n")
	fields := strings.Fields(outString)
	lines, objFileStr := findDepSlice(fields)
	objFile := NewObjFileString(objFileStr, relPath)
	objFile.Deps = append(objFile.Deps, lines...)
	return objFile
}

func parseCFlagsEntry(l string, exts []string, excludeSubdirs []string) (*CflagEntry, error) {
	trimStr := strings.TrimPrefix(l, "-I")
	trimStr += "/"
	var cflagEntry CflagEntry
	cflagEntry.Include = trimStr
	cflagEntry.ObjFiles = make([]*ObjFile, 0)
	err := filepath.WalkDir(trimStr, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		pObjFile := parseFileEntry(trimStr, path, d, exts, excludeSubdirs)
		if pObjFile != nil {
			cflagEntry.ObjFiles = append(cflagEntry.ObjFiles, pObjFile)
		}
		return nil
	})
	sort.Slice(cflagEntry.ObjFiles, func(i, j int) bool {
		if cflagEntry.ObjFiles[i].OFile == "" {
			return false
		}
		return len(cflagEntry.ObjFiles[i].Deps) > len(cflagEntry.ObjFiles[j].Deps)
	})
	return &cflagEntry, err
}

func sortIncludes(expandCflags string, cfg *LLCppConfig, exts []string, excludeSubdirs []string) {
	list := strings.Fields(expandCflags)
	cflagEntryList := make([]*CflagEntry, 0)
	for _, l := range list {
		pCflagEntry, err := parseCFlagsEntry(l, exts, excludeSubdirs)
		if err != nil {
			log.Panic(err)
		}
		if pCflagEntry != nil {
			cflagEntryList = append(cflagEntryList, pCflagEntry)
		}
	}
	cfg.Include = make([]string, 0)
	for _, cflagEntry := range cflagEntryList {
		depCtx := NewDepCtx(cflagEntry)
		for _, objFile := range cflagEntry.ObjFiles {
			depCtx.ExpandDeps(objFile)
		}
		for _, objFile := range cflagEntry.ObjFiles {
			cfg.Include = append(cfg.Include, objFile.HFile)
			expandDepIds := depCtx.depsMap[objFile]
			for _, depId := range expandDepIds {
				depObj := depCtx.GetObjFileById(depId)
				cfg.Include = append(cfg.Include, depObj.HFile)
			}
		}
	}
	cfg.Include = removeDupFilePath(cfg.Include)
}

func NewLLCppConfig(name string, isCpp bool) *LLCppConfig {
	cfg := &LLCppConfig{
		Name: name,
	}
	cfg.CFlags = fmt.Sprintf("$(pkg-config --cflags %s)", name)
	cfg.Libs = fmt.Sprintf("$(pkg-config --libs %s)", name)
	cfg.TrimPrefixes = []string{}
	cfg.Cplusplus = isCpp
	return cfg
}

func NormalizePackageName(name string) string {
	fields := strings.FieldsFunc(name, func(r rune) bool {
		return !unicode.IsLetter(r) && r != '_' && !unicode.IsDigit(r)
	})
	if len(fields) > 0 {
		if len(fields[0]) > 0 && unicode.IsDigit(rune(fields[0][0])) {
			fields[0] = "_" + fields[0]
		}
	}
	return strings.Join(fields, "_")
}

func GenCfg(name string, cpp bool, expand CfgMode, exts []string, excludeSubdirs []string) (*bytes.Buffer, error) {
	if len(name) == 0 {
		return nil, NewEmptyStringError("name")
	}
	cfg := NewLLCppConfig(name, cpp)
	switch expand {
	case ExpandMode:
		expandCFlagsAndLibs(name, cfg, "")
		sortIncludes(cfg.CFlags, cfg, exts, excludeSubdirs)
	case SortMode:
		expandCflags, _ := ExpandName(name, "", "cflags")
		sortIncludes(expandCflags, cfg, exts, excludeSubdirs)
	case NormalMode:
		cfg.Include, cfg.CFlags, _ = ExpandCFlagsName(name)
	}

	cfg.Name = NormalizePackageName(cfg.Name)

	if runtime.GOOS == LINUX {
		libpath, _ := SearchLib(name)
		if len(libpath) > 0 {
			libs, err := CmdOutString(ExecCommand("pkg-config", "--libs", name), "")
			if err == nil {
				cfg.Libs = fmt.Sprintf("-L%s %s", libpath, strings.TrimSpace(libs))
			}
		}
	}

	buf := bytes.NewBuffer([]byte{})
	jsonEncoder := json.NewEncoder(buf)
	jsonEncoder.SetIndent("", "\t")
	err := jsonEncoder.Encode(cfg)
	if err != nil {
		return nil, err
	}
	return buf, nil
}
