llcppg - LLGo autogen tool for C/C++ libraries
====

[![Build Status](https://github.com/goplus/llcppg/actions/workflows/go.yml/badge.svg)](https://github.com/goplus/llcppg/actions/workflows/go.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/goplus/llcppg)](https://goreportcard.com/report/github.com/goplus/llcppg)
[![Coverage Status](https://codecov.io/gh/goplus/llcppg/branch/main/graph/badge.svg)](https://codecov.io/gh/goplus/llcppg)
[![Language](https://img.shields.io/badge/language-Go+-blue.svg)](https://github.com/goplus/gop)

## How to install

This project depends on LLGO's C ecosystem integration capability, and some components of this tool must be compiled with LLGO. For LLGO installation, please refer to:
https://github.com/goplus/llgo?tab=readme-ov-file#how-to-install

```bash
brew install cjson # macos
apt-get install libcjson-dev # linux
llgo install ./_xtool/llcppsymg
llgo install ./_xtool/llcppsigfetch
go install ./cmd/llcppcfg
go install ./cmd/gogensig
go install .
```


## Usage

llcppg.cfg file is a configure file used by llcppg. Once llcppg.cfg is generated then you can run llcppg command to generate go pacakge for the c/c++ lib.

```sh
llcppg [config-file]
```

If `config-file` is not specified, a `llcppg.cfg` file is used in current directory. 
Here's a demo configuration to generate LLGO bindings for cjson library:

```json
{
  "name": "cjson",
  "cflags": "$(pkg-config --cflags libcjson)",
  "include": ["cJSON.h","cJSON_Utils.h"],
  "libs": "$(pkg-config --libs libcjson libcjson_utils)",
  "trimPrefixes": ["cJSONUtils_","cJSON_"]
}
```

After creating the configuration file, run:

```bash
llcppg llcppg.cfg
```

After execution, a Go project will be generated in a directory named after the config name (which is also the package name). For example, with the cjson configuration above, you'll see:

```bash
cjson/
├── cJSON.go
├── cJSON_Utils.go
├── cjson_autogen_link.go
├── llcppg.pub
├── go.mod
└── go.sum
```

Import the generated cjson package and try this demo:

```go
package main

import (
    "cjson"
    "github.com/goplus/llgo/c"
)

func main() {
	mod := cjson.CreateObject()
	mod.AddItemToObject(c.Str("hello"), cjson.CreateString(c.Str("llgo")))
	mod.AddItemToObject(c.Str("hello"), cjson.CreateString(c.Str("llcppg")))
	cstr := mod.PrintUnformatted()
	c.Printf(c.Str("%s\n"), cstr)
}
```
Run the demo with `llgo run .`, you will see the following output:
```
{"hello":"llgo","hello":"llcppg"}
```

### Generated Bindings

You can see that functions from the C header files have been automatically mapped and converted to corresponding LLGO binding functions. 

Original C function:
```c
CJSON_PUBLIC(cJSON *) cJSON_CreateObject(void);
```

Converted to LLGO binding function:
```go
//go:linkname CreateObject C.cJSON_CreateObject
func CreateObject() *CJSON 
```

The types defined in the header file are also converted, for example the cJSON struct:

Original C struct:
```c
typedef struct cJSON
{
    struct cJSON *next;
    struct cJSON *prev;
    struct cJSON *child;
    int type;
    char *valuestring;
    int valueint;
    double valuedouble;
    char *string;
} cJSON;
```

Converted to Go struct:
```go
type CJSON struct {
	Next        *CJSON
	Prev        *CJSON
	Child       *CJSON
	Type        c.Int
	Valuestring *int8
	Valueint    c.Int
	Valuedouble float64
	String      *int8
}
```

Notably, to make the API more idiomatic in Go, when a C function's first parameter is a converted type (like cJSON *), the function is automatically converted into a method of that type.

Original C function:
```c
CJSON_PUBLIC(cJSON_bool) cJSON_AddItemToArray(cJSON *array, cJSON *item);
```

Converted to Go method:
```go
// llgo:link (*CJSON).AddItemToObject C.cJSON_AddItemToObject
func (p *CJSON) AddItemToObject(string *int8, item *CJSON) CJSONBool {
	return 0
}
```

You can also observe the corresponding type name transformations. The generated `llcppg.pub` file contains a mapping table from C types to Go type names (which will be used for package dependency handling). For the example above, the `llcppg.pub` file looks like this, where the first field on the left is the C type and the first field on the right is the corresponding Go type name.
```
cJSON CJSON
cJSON_Hooks Hooks
cJSON_bool Bool
```
 You can customize these type mappings by editing this file (see [Customizing Bindings](#type-customization)).

### Customizing Bindings
#### Function Customization
When you run llcppg directly with the above configuration, it will generate function names according to the configuration. After execution, you'll find a `llcppg.symb.json` file in the current directory. 

```json
[
  {
    "mangle": "cJSON_CreateObject",
    "c++": "cJSON_CreateObject()",
    "go": "CreateObject"
  },
  {
    "mangle": "cJSON_AddItemToObject",
    "c++": "cJSON_AddItemToObject(cJSON *, const char *, cJSON *)",
    "go": "(*CJSON).AddItemToObject"
  },
  {
    "mangle": "cJSON_PrintUnformatted",
    "c++": "cJSON_PrintUnformatted(const cJSON *)",
    "go": "(*CJSON).PrintUnformatted"
  },
]
```

- `mangle` field contains the symbol name of function
- `c++` field shows the function prototype from the header file
- `go` field displays the function name that will be generated in LLGO binding. 
  
  You can customize this field to:
  1. Change function names (e.g. "CreateObject" to "Object" for simplicity)
  2. Remove the method receiver prefix to generate a function instead of a method

For example, to convert `(*CJSON).PrintUnformatted` from a method to a function, simply remove the `(*CJSON).` prefix in the configuration file:

```json
[
  {
    "mangle": "cJSON_PrintUnformatted",
    "c++": "cJSON_PrintUnformatted(const cJSON *)",
    "go": "PrintUnformatted"
  },
]
```
This will generate a function instead of a method in the Go code:
```go
//go:linkname PrintUnformatted C.cJSON_PrintUnformatted
func PrintUnformatted(item *CJSON) *int8
```

You can customize the generated bindings by modifying the `llcppg.symb.json` file.

After modifying the file, run llcppg again to apply your customized bindings.

The symbol table is generated by llcppsymg, which is internally called by llcppg to generate the symbol table as input for Go code generation. 

You can also run llcppsymg separately to customize the symbol table before running llcppg. To do this, use the command:

```sh
llcppg -symbgen
```

If you only want to generate Go code using an already generated symbol table, execute:
```sh
llcppg -codegen
```
#### Type Customization
The `llcppg.pub` file maintains type mapping relationships between C and Go. You can customize these mappings to better suit your needs.
For instance, if you prefer to use `JSON` instead of `cJSON` as the Go type name, simply modify the `llcppg.pub` file as follows:
```
cJSON JSON
```
After running llcppg again, all generated code will use the new type name. The struct definition and its methods will be automatically updated:
```go
type JSON struct {
  // .....
}
// llgo:link (*JSON).PrintBuffered C.cJSON_PrintBuffered
func (recv_ *JSON) PrintBuffered(prebuffer c.Int, fmt Bool) *int8 {
	return nil
}
```

More demo projects and configuration files can be found under `_llcppgtest` directory.

### Important Note on Header File Ordering

llcppg follows C language's dependency resolution order when processing header files. The order of files in the `includes` configuration determines the processing sequence, and incorrect ordering can lead to type resolution failures. Here's an example from the LZMA library that demonstrates the dependency relationships:

`lzma/vli.h`:
```c
typedef uint64_t lzma_vli;
```

`lzma/filter.h`:
```c
/**
 * \file        lzma/filter.h
 * \brief       Common filter related types and functions
 * \note        Never include this file directly. Use <lzma.h> instead.
 */
typedef struct {
    lzma_vli id;      // Uses lzma_vli from vli.h
    void *options;
} lzma_filter;
```

`lzma.h`:
```c
/**
 * \file        lzma.h
 * \brief       Main LZMA library header
 */
#include "lzma/vli.h"
#include "lzma/filter.h"
```

Since llcppg processes headers in the order specified in the configuration, you need to list them in the correct dependency order:

✅ Correct ordering:
```json
{
   "includes": ["lzma.h", "lzma/vli.h", "lzma/filter.h"]
}
```

❌ Incorrect ordering:
```json
{
   "includes": ["lzma/filter.h", "lzma.h", "lzma/vli.h"]
}
```

The incorrect ordering will fail because `filter.h` uses the `lzma_vli` type before it's defined.

#### Translation Unit Processing

When processing headers with the correct ordering, llcppg employs a caching strategy:

1. First, it creates a translation unit starting from `lzma.h`. During this process:
   - It follows normal C language parsing order for includes
   - When processing `lzma/vli.h` (included by `lzma.h`), the `lzma_vli` type definition is cached in this translation unit
   - When it reaches `lzma/filter.h`, the type references work correctly because `lzma_vli` is already defined in this translation unit

2. For subsequent includes in the configuration:
   - When processing `lzma/vli.h`, llcppg creates a new translation unit
   - When reaching `lzma/filter.h`, if it was already processed in a previous translation unit (like the one starting from `lzma.h`), llcppg will use the cached version that has proper type resolution

This caching strategy ensures that type references are properly maintained when headers are processed in the correct order.

### Development Tools

### llcppcfg - Configuration Generator

```sh
llcppcfg [libname]
```

llcppcfg tool is used to generate llcppg.cfg file.

## Design

See [llcppg Design](design.md).
