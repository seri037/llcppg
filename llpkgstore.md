# llpkgstore documentation

## Directory structure

```
+ {CLibraryName}
   |
   +-- {NormalGoModuleFiles}
   |
   +-- llpkg.cfg
   |    
   +-- llcppg.cfg
   |
   +-- llcppg.symb.json
   |
   +-- llcppg.pub
   |
   +-- _demo
         |
         +-- {DemoName1}
         |       |
         |       +-- main.go
         |       |
         |       +-- {OptionalSubPkgs}
         |
         +-- {DemoName2}
         |       |
         |       +-- ...
         |
         +-- ...
```

- `llpkg.cfg`: definition of an llpkg's generation workflow
- `llcppg.cfg`, `llcppg.symb.json`, `llcppg.pub`: config files of `llcppg`
- `_demo`: tests to verify if llpkg can be imported, compiled and run as expected.

## llpkg.cfg Structure

```json
{
  "upstream": {
    "installer": "conan",
    "config": {
      "options": ""
    },
    "package": {
      "name": "cjson",
      "version": "1.7.18",
    }
  },
  "generator": {
    "name": "llcppg",
    "version": "0.9.7"
  }
}
```

### Field description

**package**

| key         | type    | default value   | optional | meaning           |
| -------------- | ------- | -------- | -------- | -------------- |
| name           | `string`  |        | ❌       | package name       |
| cVersion        | `string`  | \<latest\>       | ✅       | original c package version    |
| moduleVersion | `string` |  v1.0.0  | ✅ | llgo module version |

**upstream**

| key         | type    | default value   | optional | meaning           |
| -------------- | ------- | -------- | -------- | -------------- |
| name           | `string`  | "conan"       | ✅       | upstream package platform   |
| config        | `map[string]string`  | []       | ✅       | platform CLI option |

**toolchain**

| key         | type    | default value   | optional | meaning           |
| -------------- | ------- | -------- | -------- | -------------- |
| name           | `string`  | "llcppg"       | ✅       | toolchain name  |
| version        | `string`  | "latest" | ✅       | toolchain version   |

#### For developers

If no `cVersion` is specified, the `conan search` command will be used to fetch all available versions of the current C package. You can then manually select the version from the command line.

If no `moduleVersion` is specified, it will **currently** default to `v1.0.0`. This may cause conflicts with existing tags in the current repository. Please better fill it by yourself.

## Getting an llpkg

Use `llgo get` to get an llpkg:

```bash
llgo get clib@cversion
```

*e.g.* `llgo get cjson@1.7.18`

- `clib`: the original library name in C
- `cversion`: the original version in C

`llgo get` automatically handles two things:

1. Prepends required prefixes to `clib` references, converting them into valid `module_path` identifiers.
2. Convert `cversion` to canonical `module_version` using the version mapping table.

Or you can use `llgo` with go module syntax directly:

```bash
llgo get module_path@module_version
```

*e.g.* `llgo get github.com/goplus/llpkg/cjson@v1.0.0`

```bash
llgo get clib[@latest]
llgo get module_path[@latest]
```

The optional `latest` identifier is supported as a valid `cversion` or `module_version`. When `llgo get clib@latest`, `llgo get` will firstly convert `clib` to `module_path`, and then process it as `module_path@latest`. `llgo get` will find the latest llpkg and pull it.

Wrong usage:

```bash
llgo get clib@module_version
llgo get module_path@cversion
```

It's the format of the part before `@` that determines the how `llgo get` will handle the version; that is, `llgo get` will firstly check if it's a `clib`. If it is, the whole argument will be processed as `clib@cversion`; otherwise, it will be processed as `module_path@module_version`.

> **Details of `llgo get`**
>
>  1. `llgo` automatically resolves `clib@cversion` syntax into canonical `module_path@module_version` format.
>  2. Pull the go module by `go get`.
>  3. Check `llpkg.cfg` to determine if it's an llpkg. If it is:
>
>     - Run `conan install` to install binaries. `.pc` files for building will be stored in `${LLGOMODCACHE}`.
>     - Indicate the original `cversion` by adding a comment in `go.mod`. (We ignore indirect dependencies for now.)
>
>       ```
>       // go.mod 
>       require (
>             github.com/goplus/llpkg/cjson v1.1.0  // cjson 1.7.18
>       )
>       ```

## Listing clib version mapping

```
llgo list -m [-versions] [-json] [clibs/modules]
```

- `llgo list -m` is compatible with `go list -m`
- `clibs`: a set of space-separated clib[@cversion]
- `modules`: a set of space-separated module_path[@module_version]
 
You can use `clibs` as the argument. It'll print the module path and the current version mapping.

*e.g.* `llgo list -m cjson`:

```
github.com/goplus/llpkg/cjson 1.3/v0.1.1

```

*e.g.* `llgo list -m -versions cjson`:

```
github.com/goplus/llpkg/cjson 1.3/{v0.1.0 v0.1.1} 1.3.1/{v0.2.0}
```

When using `modules`, it follows the results of `go list`.

*e.g.* `llgo list -m -versions github.com/goplus/llpkg/cjson`:

```
github.com/goplus/llpkg/cjson v0.1.0 v0.1.1 v0.2.0
```

You can also use both of them in one command.

*e.g.* `llgo list -m -versions cjson github.com/goplus/llpkg/cjson`:

```
github.com/goplus/llpkg/cjson 1.3/[v0.1.0 v0.1.1] 1.3.1/[v0.2.0]
github.com/goplus/llpkg/cjson v0.1.0 v0.1.1 v0.2.0
```

Or you can also view the info in json format.

*e.g.* `llgo list -m -versions -json cjson`:

```go
type VersionMapping struct {
  CLibVersion  string   `json:"c"`
  GoModuleVersion []string `json:"go"`
}

type LLPkg struct {
  GoModule         Module           `json:"gomodule"`
  CLibVersion      string           `json:"cversion"`
  VersionMappings  []VersionMapping `json:"versions"`
} `json:"llpkg"`
```

```json
{
  "llpkg": {
    "gomodule": {}, // refer to https://go.dev/ref/mod#go-list-m
    "cversion": ,
    "versions": 
  }
}
```

## Publication via GitHub Action

### Workflow

1. Create PR to trigger GitHub Action
2. PR verification
3. llpkg generation
4. Run test
5. Review generated llpkg
6. Merge PR
7. Add a version tag by Github Action on main branch

### PR verification workflow
1. Ensure that there is only one `llpkg.cfg` file across all directories. If multiple instances of `llpkg.cfg` are detected, the PR will be aborted.  
2. Check if the directory name is valid, the directory name in PR **SHOULD** equal to `Package.Name` field in the `llpkg.cfg` file.

### llpkg generation

A standard method for generating valid llpkgs:
1. Receive binaries/headers from [upstream](#llpkgcfg-structure), and index them into `.pc` files
2. Automatically generate llpkg using a [toolchain](#llpkgcfg-structure) for different platforms
3. Combine generated results into one Go module
4. Debug and re-generate llpkg by modifying the configuration file

### Version tag rule
1. Follow Go's version management for nested modules. Tag `{CLibraryName}/{MappingVersion}` for each version.
2. This design is fully compatible with native Go modules
    ```
    github.com/goplus/llpkg/cjson@v1.7.18
    ```

### Legacy version maintenance workflow

1. Create an issue to specify which package needs to be maintained.
2. Discuss whether it should be maintained or not.
3. If maintenance is decided, close the issue and add the label `maintain:{CLibraryName}/{Version}` to trigger the GitHub Action.
4. The GitHub Action will [create a branch](#rule) from the tag if the branch dones't exist.
5. Create a maintenance pull request (PR) for the branch and re-run the [workflow](#workflow).

#### Issue format

The title of a legacy version maintenance issue **MUST** follow the format: `Maintenance: {CLibraryName}/{Version}`.  

GitHub Action will be triggered only when the issue that match this specified format is closed.

## Version conversion rules [wip]

We use a mapping table to convert C library versions to llpkg versions.

### Initial version

If the C library is stable, then start with `v1.0.0` (cjson@1.7.18)
  
Otherwise, start with `v0.1.0`, until it releases a stable version. (libass@0.17.3)
  
### Bumping rules

| Component | Trigger Condition | Example |
|-----------|--------------------|---------|
| **MAJOR** | Breaking changes introduced by upstream C library updates. | `cjson@1.7.18` → `1.0.0`, `cjson@2.0` → `2.0.0` |
| **MINOR** | Non-breaking upstream updates (features/fixes). | `cjson@1.7.19` (vs `1.7.18`) → `1.1.0`; `cjson@1.8.0` → `1.2.0` |
| **PATCH** | llpkg internal fixes **unrelated** to upstream changes, or upstream patches on history versions (see [this](#prohibition-of-legacy-patch-maintenance)). | `llpkg@1.0.0` → `1.0.1` |

- Currently, we only consider C library updates since the first release of an llpkg.
- Pre-release versions of C library like `v1.2.3-beta.2` would not be accepted.
- **Note**: Please note that the version number of the llpkg is **not related** to the version number of the C library. It's the llpkg's MINOR update that corresponds to the C library's PATCH update, while the llpkg's PATCH update is used for indicating llpkg's self-updating.

### Branch maintenance strategy

#### Context

- Existing repository tracks upstream `cjson@1.6` with historical versions: `cjson@1.5.7`, `cjson@1.5.6`, `cjson@1.6`.  
- Upstream releases `1.5.8` targeting older `1.5.x` series.

#### Rule

`1.5.8` **cannot** be merged into `main` branch (currently tracking `1.6`). Instead, we should create a new branch `release-branch.cjson/v1.5` and commit to it.

### Prohibition of legacy patch maintenance

#### Problem

| C Library Version | llpkg Version | Issue |
|--------------------|---------------|-------|
| 1.5.1             | `1.0.0`       | Initial release |
| 1.5.1 (llpkg fix) | `1.0.1`       | Patch increment |
| 1.6               | `1.1.0`       | Minor increment |
| 1.5.2             | ?             | Conflict: `1.1.0` already allocated |

If we increment PATCH to `1.0.2` to represent `cjson@1.5.2`:

| C Library Version | llpkg Version | Issue |
|--------------------|---------------|-------|
| 1.5.1             | `1.0.0`       | Initial release |
| 1.5.1 (llpkg fix) | `1.0.1`       | Patch increment |
| 1.6               | `1.1.0`       | Minor increment |
| 1.5.2             | `1.0.2`       | Conflict: `1.1.0` already allocated |
| 1.5.1 (llpkg fix 2) | `1.0.3`       | Patch increment |

`cjson@1.5.2` > `cjson@1.5.1` maps to `llpkg@1.0.2` < `llpkg@1.0.3` (breaking version ordering), which causes MVS to prioritize `1.0.3` (lower priority upstream version) over `1.0.2`.

#### Conflict resolution rule

When upstream releases patch updates for **previous minor versions**:
- NO further patches shall be applied to earlier upstream patch versions
- ALL maintenance MUST target the **newest upstream patch version**

#### Rationale

New patch updates from upstream naturally replace older fixes. Keeping old patch versions creates unnecessary differences that don't align with SemVer principles **and may leave security vulnerabilities unpatched**.

#### Workflow

- cjson@1.5.8 released → llpkg MUST update from latest 1.5.x baseline (1.5.7)
- Original cjson@1.5.1 branch becomes immutable

### Mapping file structure

`llpkgstore.json`:

```json
{
    "cgood": {
        "versions" : [{
            "c": "1.3",
            "go": ["v0.1.0", "v0.1.1"]
        }, 
        {
            "c": "1.3.1",
            "go": ["v1.1.0"]
        }]
    }
}
```

- `c`: the original C library version.
- `go`: the converted version.

We have to consider about the module regenerating due to [toolchain](#llpkgcfg-structure) upgrading, hence, the relationship between the original C library version and the mapping version is one-to-many.

`llgo get` is expected to select the latest version from the `go` field.

## llpkg.goplus.org

This service is hosted by GitHub Pages, and the `llpkgstore.json` file is located in the same branch as GitHub Pages. When running `llgo get`, it will download the file to `LLGOMODCACHE`.

### Function

1. Provide a download of the mapping table.
2. Provide version queries for Go Modules corresponding to C libraries.
3. Provide links to specific C libraries on Conan.io.
4. Preview the structure of `llpkg.cfg`.

### Router

1. `/`: Home page with a search bar at the top and multiple llpkgs. Users can search for llpkgs by name and view the latest two versions. Clicking an llpkg opens a modal displaying:
   - Information about the original C library on Conan
   - A preview of its `llpkg.cfg`
   - All available versions of the llpkg
2. `/llpkgstore.json`: Provides the mapping table download.

**Note**: llpkg details are displayed in modals instead of new pages, as `llpkgstore.json` is loaded during the initial homepage access and does not require additional requests.

### Interaction with web service

When executing `llgo get clib@cversion`, a series of actions will be performed to convert `cversion` to `module_version`:
1. Fetch the latest `llpkgstore.json`
2. Parse the JSON file to find the corresponding `module_version` array
3. Select the latest patched version from the array
3. Retrieve llpkg

## `LLGOMODCACHE`

One usage is to store `.pc` files of the C library and allow `llgo build` to find them.

1. if `LLGOMODCACHE` is empty, it defaults to `${HOME}/llgo/pkg/mod`.
2. `{LLGOMODCACHE}/{module_path}@{module_version}/pkg-config` stores `.pc` files of C libs needed by llpkg.
