package convert

import (
	"bytes"
	"fmt"
	"go/token"
	"go/types"
	"log"
	"os"
	"path/filepath"

	"github.com/goplus/gogen"
	"github.com/goplus/llcppg/ast"
	cfg "github.com/goplus/llcppg/cmd/gogensig/config"
	"github.com/goplus/llcppg/cmd/gogensig/convert/names"
	"github.com/goplus/llcppg/cmd/gogensig/errs"
	"github.com/goplus/mod/gopmod"
)

const (
	DbgFlagAll = 1
)

var (
	debug bool
)

func SetDebug(flags int) {
	if flags != 0 {
		debug = true
	}
}

// In Processing Package
type Package struct {
	*PkgInfo
	p       *gogen.Package // package writer
	conf    *PackageConfig // package config
	cvt     *TypeConv      // package type convert
	curFile *HeaderFile    // current processing c header file.

	// incomplete stores type declarations that are not fully defined yet.
	// This is used to handle forward declarations and self-referential types in C/C++.
	incomplete map[string]*gogen.TypeDecl // origin name(in c) -> TypeDecl

	// deferTypes stores type declarations that need to be resolved later.
	// The key is the Go type declaration, and the value is a function that will return
	// the actual type when called.
	// These functions will be executed after all incomplete types are initialized.
	//
	// This is particularly important when handling typedef of incomplete types in Go.
	// In Go, when creating a new type via "type xxx xxx", the underlying type must exist.
	// However, when typedef-ing an incomplete type, its underlying type doesn't exist yet.
	// For such cases, we defer the type initialization until after all incomplete types
	// are properly initialized, at which point we can correctly reference the underlying type.
	deferTypes map[*gogen.TypeDecl]func() (types.Type, error)

	nameMapper *names.NameMapper // handles name mapping and uniqueness
}

type PackageConfig struct {
	PkgBase
	Name        string // current package name
	OutputDir   string
	SymbolTable *cfg.SymbolTable
	GenConf     *gogen.Config
}

// When creating a new package for conversion, a Go file named after the package is generated by default.
// If SetCurFile is not called, all type conversions will be written to this default Go file.
func NewPackage(config *PackageConfig) *Package {
	p := &Package{
		p:          gogen.NewPackage(config.PkgPath, config.Name, config.GenConf),
		conf:       config,
		incomplete: make(map[string]*gogen.TypeDecl),
		deferTypes: make(map[*gogen.TypeDecl]func() (types.Type, error)),
		nameMapper: names.NewNameMapper(),
	}

	mod, err := gopmod.Load(config.OutputDir)
	if err != nil {
		log.Panicf("failed to load mod: %s", err.Error())
	}

	p.PkgInfo = NewPkgInfo(config.PkgPath, config.OutputDir, config.CppgConf, config.Pubs)
	for name, goName := range config.Pubs {
		p.nameMapper.SetMapping(name, goName)
	}

	pkgManager := NewPkgDepLoader(mod, p.p)
	err = pkgManager.InitDeps(p.PkgInfo)
	if err != nil {
		log.Panicf("failed to init deps: %s", err.Error())
	}

	clib := p.p.Import("github.com/goplus/llgo/c")
	typeMap := NewBuiltinTypeMapWithPkgRefS(clib, p.p.Unsafe())
	p.cvt = NewConv(&TypeConfig{
		Types:       p.p.Types,
		TypeMap:     typeMap,
		SymbolTable: config.SymbolTable,
		Package:     p,
	})
	err = p.SetCurFile(p.conf.Name, "", false, false, false)
	if err != nil {
		log.Panicf("failed to set current file: %s", err.Error())
	}
	return p
}

func (p *Package) SetCurFile(file string, incPath string, isHeaderFile bool, inCurPkg bool, isSys bool) error {
	curHeaderFile, err := NewHeaderFile(file, incPath, isHeaderFile, inCurPkg, isSys)
	if err != nil {
		return err
	}
	p.files = append(p.files, curHeaderFile)
	p.curFile = curHeaderFile
	fileName := p.curFile.ToGoFileName()
	if debug {
		log.Printf("SetCurFile: %s File in Current Package: %v\n", fileName, inCurPkg)
	}
	if _, err := p.p.SetCurFile(fileName, true); err != nil {
		return fmt.Errorf("fail to set current file %s\n%w", file, err)
	}
	p.p.Unsafe().MarkForceUsed(p.p)
	return nil
}

func (p *Package) GetGenPackage() *gogen.Package {
	return p.p
}

func (p *Package) GetOutputDir() string {
	return p.conf.OutputDir
}

func (p *Package) GetTypeConv() *TypeConv {
	return p.cvt
}

// todo(zzy):refine logic
func (p *Package) linkLib(lib string) error {
	if lib == "" {
		return fmt.Errorf("empty lib name")
	}
	linkString := fmt.Sprintf("link: %s;", lib)
	p.p.CB().NewConstStart(types.Typ[types.String], "LLGoPackage").Val(linkString).EndInit(1)
	return nil
}

func (p *Package) newReceiver(typ *ast.FuncType) *types.Var {
	recvField := typ.Params.List[0]
	recvType, err := p.ToType(recvField.Type)
	if err != nil {
		log.Println(err)
	}
	return p.p.NewParam(token.NoPos, "p", recvType)
}

func (p *Package) ToSigSignature(goFuncName *GoFuncName, funcDecl *ast.FuncDecl) (*types.Signature, error) {
	var sig *types.Signature
	var recv *types.Var
	var err error
	if goFuncName.HasReceiver() &&
		funcDecl.Type.Params.List != nil &&
		len(funcDecl.Type.Params.List) > 0 {
		recv = p.newReceiver(funcDecl.Type)
	}
	sig, err = p.cvt.ToSignature(funcDecl.Type, recv)
	if err != nil {
		return nil, err
	}
	return sig, nil
}

func (p *Package) bodyStart(decl *gogen.Func, ret ast.Expr) error {
	if !Expr(ret).IsVoid() {
		retType, err := p.ToType(ret)
		if err != nil {
			return err
		}
		decl.BodyStart(p.p).ZeroLit(retType).Return(1).End()
	} else {
		decl.BodyStart(p.p).End()
	}
	return nil
}

func (p *Package) newFuncDeclAndComment(goFuncName *GoFuncName, sig *types.Signature, funcDecl *ast.FuncDecl) error {
	var decl *gogen.Func
	if goFuncName.HasReceiver() {
		decl = p.p.NewFuncDecl(token.NoPos, goFuncName.funcName, sig)
		err := p.bodyStart(decl, funcDecl.Type.Ret)
		if err != nil {
			return err
		}
	} else {
		decl = p.p.NewFuncDecl(token.NoPos, goFuncName.OriginGoSymbolName(), sig)
	}
	doc := CommentGroup(funcDecl.Doc)
	doc.AddCommentGroup(NewFuncDocComments(funcDecl.Name.Name, goFuncName.OriginGoSymbolName()))
	decl.SetComments(p.p, doc.CommentGroup)
	return nil
}

func (p *Package) NewFuncDecl(funcDecl *ast.FuncDecl) error {
	skip, anony, err := p.cvt.handleSysType(funcDecl.Name, funcDecl.Loc, p.curFile.sysIncPath)
	if skip {
		if debug {
			log.Printf("NewFuncDecl: %v is a function of system header file\n", funcDecl.Name)
		}
		return err
	}
	if debug {
		log.Printf("NewFuncDecl: %v\n", funcDecl.Name)
	}
	if anony {
		return errs.NewAnonymousFuncNotSupportError()
	}

	goSymbolName, err := p.cvt.LookupSymbol(funcDecl.MangledName)
	if err != nil {
		// not gen the function not in the symbolmap
		return err
	}
	if obj := p.p.Types.Scope().Lookup(goSymbolName); obj != nil {
		return errs.NewFuncAlreadyDefinedError(goSymbolName)
	}
	goFuncName := NewGoFuncName(goSymbolName)
	sig, err := p.ToSigSignature(goFuncName, funcDecl)
	if err != nil {
		return err
	}
	return p.newFuncDeclAndComment(goFuncName, sig, funcDecl)
}

// NewTypeDecl converts C/C++ type declarations to Go.
// Besides regular type declarations, it also supports:
// - Forward declarations: Pre-registers incomplete types for later definition
// - Self-referential types: Handles types that reference themselves (like linked lists)
func (p *Package) NewTypeDecl(typeDecl *ast.TypeDecl) error {
	skip, anony, err := p.cvt.handleSysType(typeDecl.Name, typeDecl.Loc, p.curFile.sysIncPath)
	if skip {
		if debug {
			log.Printf("NewTypeDecl: %s type of system header\n", typeDecl.Name)
		}
		return err
	}
	if debug {
		log.Printf("NewTypeDecl: %v\n", typeDecl.Name)
	}
	if anony {
		if debug {
			log.Println("NewTypeDecl:Skip a anonymous type")
		}
		return nil
	}

	cname := typeDecl.Name.Name
	isForward := p.cvt.inComplete(typeDecl.Type)
	name, changed, err := p.DeclName(cname)
	if err != nil {
		if isForward {
			return nil
		}
		return err
	}
	p.CollectNameMapping(cname, name)

	decl := p.handleTypeDecl(name, cname, typeDecl)

	if changed {
		substObj(p.p.Types, p.p.Types.Scope(), cname, decl.Type().Obj())
	}

	if !isForward {
		if err := p.handleCompleteType(decl, typeDecl.Type, cname); err != nil {
			return err
		}
	}
	return nil
}

// handleTypeDecl creates a new type declaration or retrieves existing one
func (p *Package) handleTypeDecl(pubname string, cname string, typeDecl *ast.TypeDecl) *gogen.TypeDecl {
	if existDecl, exists := p.incomplete[cname]; exists {
		return existDecl
	}
	decl := p.emptyTypeDecl(pubname, typeDecl.Doc)
	if p.cvt.inComplete(typeDecl.Type) {
		p.incomplete[cname] = decl
	}
	return decl
}

func (p *Package) handleCompleteType(decl *gogen.TypeDecl, typ *ast.RecordType, name string) error {
	defer delete(p.incomplete, name)
	structType, err := p.cvt.RecordTypeToStruct(typ)
	if err != nil {
		// For incomplete type's conerter error, we use default struct type
		decl.InitType(p.p, types.NewStruct(p.cvt.defaultRecordField(), nil))
		return err
	}
	decl.InitType(p.p, structType)
	return nil
}

// handleImplicitForwardDecl handles type references that cannot be found in the current scope.
// For such declarations, create a empty type decl and store it in the
// incomplete map, but not in the public symbol table.
func (p *Package) handleImplicitForwardDecl(name string) *gogen.TypeDecl {
	pubName := p.nameMapper.GetGoName(name, p.trimPrefixes())
	decl := p.emptyTypeDecl(pubName, nil)
	p.incomplete[name] = decl
	p.nameMapper.SetMapping(name, pubName)
	return decl
}

func (p *Package) emptyTypeDecl(name string, doc *ast.CommentGroup) *gogen.TypeDecl {
	typeBlock := p.p.NewTypeDefs()
	typeBlock.SetComments(CommentGroup(doc).CommentGroup)
	return typeBlock.NewType(name)
}

func (p *Package) NewTypedefDecl(typedefDecl *ast.TypedefDecl) error {
	skip, _, err := p.cvt.handleSysType(typedefDecl.Name, typedefDecl.Loc, p.curFile.sysIncPath)
	if skip {
		if debug {
			log.Printf("NewTypedefDecl: %v is a typedef of system header file\n", typedefDecl.Name)
		}
		return err
	}
	if debug {
		log.Printf("NewTypedefDecl: %v\n", typedefDecl.Name)
	}
	name, changed, err := p.DeclName(typedefDecl.Name.Name)
	if err != nil {
		return err
	}
	p.CollectNameMapping(typedefDecl.Name.Name, name)

	genDecl := p.p.NewTypeDefs()
	typeSpecdecl := genDecl.NewType(name)

	if changed {
		substObj(p.p.Types, p.p.Types.Scope(), typedefDecl.Name.Name, typeSpecdecl.Type().Obj())
	}

	if tagRef, ok := typedefDecl.Type.(*ast.TagExpr); ok {
		inc := p.handleTyperefIncomplete(tagRef, typeSpecdecl)
		if inc {
			return nil
		}
	}

	typ, err := p.ToType(typedefDecl.Type)
	if err != nil {
		typeSpecdecl.InitType(p.p, types.NewStruct(p.cvt.defaultRecordField(), nil))
		return err
	}

	typeSpecdecl.InitType(p.p, typ)
	if _, ok := typ.(*types.Signature); ok {
		genDecl.SetComments(NewTypecDocComments())
	}

	return nil
}

func (p *Package) handleTyperefIncomplete(tagRef *ast.TagExpr, typeSpecdecl *gogen.TypeDecl) bool {
	var name string
	switch n := tagRef.Name.(type) {
	case *ast.Ident:
		name = n.Name
	case *ast.ScopingExpr:
		// todo(zzy):scoping
		panic("todo:scoping expr not supported")
	}
	_, inc := p.incomplete[name]
	if !inc {
		return false
	}
	p.deferTypes[typeSpecdecl] = func() (types.Type, error) {
		typ, err := p.ToType(tagRef)
		if err != nil {
			return nil, err
		}
		// a function type will not be a incomplete type,so we not need to check signature and add comments
		return typ, nil
	}
	return true
}

// Convert ast.Expr to types.Type
func (p *Package) ToType(expr ast.Expr) (types.Type, error) {
	return p.cvt.ToType(expr)
}

func (p *Package) NewTypedefs(name string, typ types.Type) *gogen.TypeDecl {
	def := p.p.NewTypeDefs()
	t := def.NewType(name)
	t.InitType(def.Pkg(), typ)
	def.Complete()
	return t
}

func (p *Package) NewEnumTypeDecl(enumTypeDecl *ast.EnumTypeDecl) error {
	skip, _, err := p.cvt.handleSysType(enumTypeDecl.Name, enumTypeDecl.Loc, p.curFile.sysIncPath)
	if skip {
		if debug {
			log.Printf("NewEnumTypeDecl: %v is a enum type of system header file\n", enumTypeDecl.Name)
		}
		return err
	}
	if debug {
		log.Printf("NewEnumTypeDecl: %v\n", enumTypeDecl.Name)
	}
	enumType, enumTypeName, err := p.createEnumType(enumTypeDecl.Name)
	if err != nil {
		return err
	}
	if len(enumTypeDecl.Type.Items) > 0 {
		err = p.createEnumItems(enumTypeDecl.Type.Items, enumType, enumTypeName)
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *Package) createEnumType(enumName *ast.Ident) (types.Type, string, error) {
	var name string
	var changed bool
	var err error
	var t *gogen.TypeDecl
	if enumName != nil {
		name, changed, err = p.DeclName(enumName.Name)
		if err != nil {
			return nil, "", errs.NewTypeDefinedError(name, enumName.Name)
		}
		p.CollectNameMapping(enumName.Name, name)
	}
	enumType := p.cvt.ToDefaultEnumType()
	if name != "" {
		t = p.NewTypedefs(name, enumType)
		enumType = p.p.Types.Scope().Lookup(name).Type()
	}
	if changed {
		substObj(p.p.Types, p.p.Types.Scope(), enumName.Name, t.Type().Obj())
	}
	return enumType, name, nil
}

func (p *Package) createEnumItems(items []*ast.EnumItem, enumType types.Type, enumTypeName string) error {
	constDefs := p.p.NewConstDefs(p.p.Types.Scope())
	for _, item := range items {
		var constName string
		// maybe get a new name,because the after executed name,have some situation will found same name
		if enumTypeName != "" {
			constName = enumTypeName + "_" + item.Name.Name
		} else {
			constName = item.Name.Name
		}
		name, changed, err := p.DeclName(constName)
		if err != nil {
			return errs.NewTypeDefinedError(name, constName)
		}
		val, err := Expr(item.Value).ToInt()
		if err != nil {
			return err
		}
		constDefs.New(func(cb *gogen.CodeBuilder) int {
			cb.Val(val)
			return 1
		}, 0, token.NoPos, enumType, name)
		if changed {
			if obj := p.p.Types.Scope().Lookup(name); obj != nil {
				substObj(p.p.Types, p.p.Types.Scope(), item.Name.Name, obj)
			}
		}
	}
	return nil
}

// WritePkgFiles writes all converted header files to Go files.
// Calls deferTypeBuild() first to complete all incomplete type definitions,
// because some types may be implemented across multiple files.
func (p *Package) WritePkgFiles() error {
	err := p.deferTypeBuild()
	if err != nil {
		return err
	}
	for _, file := range p.files {
		if file.isHeaderFile && !file.isSys {
			err := p.Write(file.file)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// Write generates a Go file based on the package content.
// The output file will be generated in a subdirectory named after the package within the outputDir.
// If outputDir is not provided, the current directory will be used.
// The header file name is the go file name.
//
// Files that are already processed in dependent packages will not be output.
func (p *Package) Write(headerFile string) error {
	fileName := names.HeaderFileToGo(headerFile)
	filePath := filepath.Join(p.GetOutputDir(), fileName)
	if debug {
		log.Printf("Write HeaderFile [%s] from  gogen:[%s] to [%s]\n", headerFile, fileName, filePath)
	}
	return p.writeToFile(fileName, filePath)
}

func (p *Package) WriteLinkFile() (string, error) {
	fileName := p.conf.Name + "_autogen_link.go"
	filePath := filepath.Join(p.GetOutputDir(), fileName)
	_, err := p.p.SetCurFile(fileName, true)
	if err != nil {
		return "", fmt.Errorf("failed to set current file: %w", err)
	}
	err = p.linkLib(p.conf.CppgConf.Libs)
	if debug {
		log.Printf("Write LinkFile [%s] from  gogen:[%s] to [%s]\n", fileName, fileName, filePath)
	}
	if err != nil {
		return "", fmt.Errorf("failed to link lib: %w", err)
	}
	if err := p.writeToFile(fileName, filePath); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}
	return filePath, nil
}

// WriteDefaultFileToBuffer writes the content of the default Go file to a buffer.
// The default file is named after the package (p.Name() + ".go").
// This method is particularly useful for testing type outputs, especially in package tests
// where there typically isn't (and doesn't need to be) a corresponding header file.
// Before calling SetCurFile, all type creations are written to this default gogen file.
// It allows for easy inspection of generated types without the need for actual file I/O.
func (p *Package) WriteDefaultFileToBuffer() (*bytes.Buffer, error) {
	return p.WriteToBuffer(p.conf.Name + ".go")
}

// Write the corresponding files in gogen package to the file
func (p *Package) writeToFile(genFName string, filePath string) error {
	buf, err := p.WriteToBuffer(genFName)
	if err != nil {
		return err
	}
	return os.WriteFile(filePath, buf.Bytes(), 0644)
}

// Write the corresponding files in gogen package to the buffer
func (p *Package) WriteToBuffer(genFName string) (*bytes.Buffer, error) {
	buf := new(bytes.Buffer)
	err := p.p.WriteTo(buf, genFName)
	if err != nil {
		return nil, fmt.Errorf("failed to write to buffer: %w", err)
	}
	return buf, nil
}

func (p *Package) deferTypeBuild() error {
	for _, decl := range p.incomplete {
		decl.InitType(p.p, types.NewStruct(p.cvt.defaultRecordField(), nil))
	}
	for decl, getTyp := range p.deferTypes {
		typ, err := getTyp()
		if typ != nil {
			decl.InitType(p.p, typ)
		}
		if err != nil {
			return err
		}
	}
	p.incomplete = make(map[string]*gogen.TypeDecl, 0)
	p.deferTypes = make(map[*gogen.TypeDecl]func() (types.Type, error), 0)
	return nil
}

func (p *Package) WritePubFile() error {
	return cfg.WritePubFile(filepath.Join(p.GetOutputDir(), "llcppg.pub"), p.Pubs)
}

// For a decl name, it should be unique
func (p *Package) DeclName(name string) (pubName string, changed bool, err error) {
	pubName, changed = p.nameMapper.GetUniqueGoName(name, p.trimPrefixes())
	// if the type is incomplete,it's ok to have the same name
	if obj := p.p.Types.Scope().Lookup(name); obj != nil && p.incomplete[name] == nil {
		return "", false, errs.NewTypeDefinedError(pubName, name)
	}
	return pubName, changed, nil
}

func (p *Package) trimPrefixes() []string {
	if p.curFile.inCurPkg {
		return p.CppgConf.TrimPrefixes
	}
	return []string{}
}

// Collect the name mapping between origin name and pubname
// if in current package, it will be collected in public symbol table
func (p *Package) CollectNameMapping(originName, newName string) {
	value := ""
	if originName != newName {
		value = newName
	}
	p.nameMapper.SetMapping(originName, value)
	if p.curFile.inCurPkg {
		p.Pubs[originName] = value
	}
}

// Return all include paths of dependent packages
func (p *Package) DepIncPaths() []string {
	visited := make(map[string]bool)
	var paths []string
	var collectPaths func(pkg *PkgInfo)
	var notFounds map[string][]string // pkgpath -> include path
	var allfailed []string            // which pkg's header file failed to find any include path

	collectPaths = func(pkg *PkgInfo) {
		for _, dep := range pkg.Deps {
			incPaths, notFnds, err := dep.GetIncPaths()
			if err != nil {
				allfailed = append(allfailed, dep.PkgPath)
			} else if len(notFnds) > 0 {
				if notFounds == nil {
					notFounds = make(map[string][]string)
				}
				notFounds[dep.PkgPath] = notFnds
			}
			for _, path := range incPaths {
				if !visited[path] {
					visited[path] = true
					paths = append(paths, path)
				}
			}
			collectPaths(dep)
		}
	}
	collectPaths(p.PkgInfo)

	if len(notFounds) > 0 {
		for pkgPath, notFnds := range notFounds {
			log.Printf("failed to find some include paths: from %s\n", pkgPath)
			log.Println(notFnds)
		}
	}
	if len(allfailed) > 0 {
		log.Println("failed to get any include paths from these package: \n", allfailed)
	}
	return paths
}
