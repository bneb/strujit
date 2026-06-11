// Package main implements strujit, a high-performance binary stream filter.
// It leverages LLVM IR generation and JIT compilation to efficiently process
// memory-mapped binary files, avoiding deserialization and reflection overhead.
package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"text/template"
)

const (
	// fstatOffsetLinux defines the st_size offset in Linux struct stat.
	fstatOffsetLinux = 48
	// fstatOffsetDarwin defines the st_size offset in macOS struct stat.
	fstatOffsetDarwin = 96
)

// Field represents a parsed struct field with its LLVM IR properties.
type Field struct {
	Name   string
	IRType string
	Idx    int
	Size   int
	Align  int
}

// Schema represents the entire evaluated struct layout.
type Schema struct {
	IR             string
	StructSize     int
	StatOffset     int
	OrderedFields  []Field
	JSONStr        string
	JSONLen        int
	PrintLoadInsts string
	PrintArgsStr   string
}

// parseSchema tokenizes the struct definition and resolves C-struct alignments.
func parseSchema(schemaStr string) (map[string]Field, Schema) {
	fields := make(map[string]Field)
	var orderedFields []Field
	var structDef []string
	offset, maxAlign := 0, 1

	for _, token := range strings.Split(schemaStr, ",") {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		parts := strings.Fields(token) // Robust against multiple spaces
		if len(parts) != 2 {
			log.Fatalf("invalid schema field definition: '%s'. Expected 'type name'", token)
		}
		
		size, align, irType := resolveTypeProps(parts[0])
		offset = applyPadding(offset, align, &structDef)

		fld := Field{parts[1], irType, len(structDef), size, align}
		fields[parts[1]] = fld
		orderedFields = append(orderedFields, fld)
		structDef = append(structDef, irType)
		offset += size

		if align > maxAlign {
			maxAlign = align
		}
	}
	offset = applyPadding(offset, maxAlign, &structDef)

	statOffset := fstatOffsetLinux
	if runtime.GOOS == "darwin" {
		statOffset = fstatOffsetDarwin
	}

	irStr := strings.Join(structDef, ", ")
	
	// Build JSON Printf internals
	var jsonParts []string
	var loadInsts []string
	var printArgs []string
	
	for _, f := range orderedFields {
		if f.IRType == "double" {
			jsonParts = append(jsonParts, fmt.Sprintf(`"%s": %%f`, f.Name))
		} else if f.IRType == "i64" {
			jsonParts = append(jsonParts, fmt.Sprintf(`"%s": %%llu`, f.Name))
		} else {
			jsonParts = append(jsonParts, fmt.Sprintf(`"%s": %%u`, f.Name))
		}
		loadInsts = append(loadInsts, fmt.Sprintf("  %%ptr_%s = getelementptr %%Struct, ptr %%el, i64 0, i32 %d\n  %%val_%s = load %s, ptr %%ptr_%s", f.Name, f.Idx, f.Name, f.IRType, f.Name))
		printArgs = append(printArgs, fmt.Sprintf("%s %%val_%s", f.IRType, f.Name))
	}
	
	rawJson := "{" + strings.Join(jsonParts, ", ") + "}\n\x00"
	jsonLen := len(rawJson)
	
	irJson := strings.ReplaceAll(rawJson, `"`, `\22`)
	irJson = strings.ReplaceAll(irJson, "\n", `\0A`)
	irJson = strings.ReplaceAll(irJson, "\x00", `\00`)
	
	return fields, Schema{
		IR:             irStr,
		StructSize:     offset,
		StatOffset:     statOffset,
		OrderedFields:  orderedFields,
		JSONStr:        irJson,
		JSONLen:        jsonLen,
		PrintLoadInsts: strings.Join(loadInsts, "\n"),
		PrintArgsStr:   strings.Join(printArgs, ", "),
	}
}

// resolveTypeProps maps Go primitives to LLVM IR primitives and sizes.
func resolveTypeProps(goType string) (int, int, string) {
	switch goType {
	case "float64":
		return 8, 8, "double"
	case "uint8":
		return 1, 1, "i8"
	case "uint64":
		return 8, 8, "i64"
	default:
		return 4, 4, "i32"
	}
}

// applyPadding computes necessary byte padding to satisfy strict C ABIs.
func applyPadding(offset, align int, defs *[]string) int {
	if offset%align != 0 {
		pad := align - (offset % align)
		*defs = append(*defs, fmt.Sprintf("[%d x i8]", pad))
		offset += pad
	}
	return offset
}

// parsePred tokenizes the condition and prepares LLVM comparison instructions.
func parsePred(pred string, fields map[string]Field) (Field, string, string) {
	parts := strings.Fields(pred)
	if len(parts) != 3 {
		log.Fatalf("invalid predicate format: '%s', expected: 'field op value'", pred)
	}
	
	fld, ok := fields[parts[0]]
	if !ok {
		log.Fatalf("predicate references unknown field: '%s'", parts[0])
	}
	
	op, valStr := parts[1], parts[2]
	
	// IR Injection prevention: ensure value is purely numeric
	floatVal, err := strconv.ParseFloat(valStr, 64)
	if err != nil {
		log.Fatalf("predicate value must be numeric to prevent IR injection: %v", err)
	}
	
	var val string
	if fld.IRType == "double" {
		val = fmt.Sprintf("%e", floatVal)
		op = resolveFloatOp(op)
	} else {
		// Verify integer parsing for integer fields
		intVal, err := strconv.ParseInt(valStr, 10, 64)
		if err != nil {
			log.Fatalf("predicate value must be an integer for field %s", fld.Name)
		}
		val = strconv.FormatInt(intVal, 10)
		op = resolveIntOp(op)
	}
	return fld, op, val
}

// resolveFloatOp maps standard operators to LLVM float comparisons.
func resolveFloatOp(op string) string {
	mapping := map[string]string{
		">": "fcmp ogt", "<": "fcmp olt", "==": "fcmp oeq",
		">=": "fcmp oge", "<=": "fcmp ole",
	}
	if irOp, ok := mapping[op]; ok {
		return irOp
	}
	log.Fatalf("unsupported float operator: %s", op)
	return ""
}

// resolveIntOp maps standard operators to LLVM integer comparisons.
func resolveIntOp(op string) string {
	mapping := map[string]string{
		">": "icmp ugt", "<": "icmp ult", "==": "icmp eq",
		">=": "icmp uge", "<=": "icmp ule",
	}
	if irOp, ok := mapping[op]; ok {
		return irOp
	}
	log.Fatalf("unsupported int operator: %s", op)
	return ""
}

// buildIR constructs the dynamic LLVM template mapping the memory buffer.
func buildIR(sch Schema, fld Field, op, val string, isJson bool) string {
	tmpl := `; strujit dynamic LLVM IR template
%Struct=type<{ {{.S.IR}} }>
@.fmt.json = private unnamed_addr constant [{{.S.JSONLen}} x i8] c"{{.S.JSONStr}}"
@.err.io = private unnamed_addr constant [20 x i8] c"strujit: I/O error\0A\00"
@.mode.w = private unnamed_addr constant [2 x i8] c"w\00"

declare i32 @open(ptr,i32,...)
declare i32 @fstat(i32,ptr)
declare ptr @mmap(ptr,i64,i32,i32,i32,i64)
declare i32 @madvise(ptr,i64,i32)
declare i64 @fwrite(ptr,i64,i64,ptr)
declare ptr @fdopen(i32,ptr)
declare i32 @fflush(ptr)
declare i32 @printf(ptr, ...)
declare i32 @dprintf(i32, ptr, ...)
declare void @exit(i32)

define i32 @main(i32 %ac,ptr %av){
entry:
  %f_ptr=getelementptr ptr,ptr %av,i64 1
  %f=load ptr,ptr %f_ptr
  %fd=call i32(ptr,i32,...) @open(ptr %f,i32 0)
  %fd_ok=icmp sge i32 %fd,0
  br i1 %fd_ok,label %stat,label %err

stat:
  %buf=alloca[144 x i8],align 8
  call i32 @fstat(i32 %fd,ptr %buf)
  %sz_ptr=getelementptr i8,ptr %buf,i64 {{.S.StatOffset}}
  %sz=load i64,ptr %sz_ptr
  %map=call ptr @mmap(ptr null,i64 %sz,i32 1,i32 2,i32 %fd,i64 0)
  %ok=icmp ne ptr %map,inttoptr(i64 -1 to ptr)
  br i1 %ok,label %init,label %err

init:
  call i32 @madvise(ptr %map,i64 %sz,i32 2)
  %stdout_ptr=alloca ptr
  %stdout_f=call ptr @fdopen(i32 1,ptr @.mode.w)
  store ptr %stdout_f,ptr %stdout_ptr
  %num=udiv i64 %sz,{{.S.StructSize}}
  %ip=alloca i64
  store i64 0,ptr %ip
  br label %cond

cond:
  %idx=load i64,ptr %ip
  %cmp=icmp ult i64 %idx,%num
  br i1 %cmp,label %body,label %exit

body:
  %el=getelementptr %Struct,ptr %map,i64 %idx
  %vp=getelementptr %Struct,ptr %el,i64 0,i32 {{.F.Idx}}
  %v=load {{.F.IRType}},ptr %vp
  %pass={{.Op}} {{.F.IRType}} %v,{{.Val}}
  br i1 %pass,label %out,label %next

out:
{{if .IsJSON}}
{{.S.PrintLoadInsts}}
  call i32 (ptr, ...) @printf(ptr @.fmt.json, {{.S.PrintArgsStr}})
{{else}}
  %f_val=load ptr,ptr %stdout_ptr
  call i64 @fwrite(ptr %el,i64 1,i64 {{.S.StructSize}},ptr %f_val)
{{end}}
  br label %next

next:
  %in=add i64 %idx,1
  store i64 %in,ptr %ip
  br label %cond

err:
  call i32 (i32, ptr, ...) @dprintf(i32 2, ptr @.err.io)
  call void @exit(i32 1)
  unreachable

exit:
  %f_val2=load ptr,ptr %stdout_ptr
  call i32 @fflush(ptr %f_val2)
  ret i32 0
}`
	f, err := os.CreateTemp("", "strujit_*.ll")
	if err != nil {
		log.Fatalf("failed to create temp IR file: %v", err)
	}
	defer f.Close()

	parsedTmpl := template.Must(template.New("ir").Parse(tmpl))
	parsedTmpl.Execute(f, map[string]any{
		"S": sch, "F": fld, "Op": op, "Val": val, "IsJSON": isJson,
	})
	return f.Name()
}

// execute triggers the LLVM JIT over the generated intermediate representation.
func execute(llFile, dataFile string) {
	// Security: To prevent PATH hijacking, explicitly check standard system paths first
	var lli string
	if _, statErr := os.Stat("/opt/homebrew/opt/llvm/bin/lli"); statErr == nil {
		lli = "/opt/homebrew/opt/llvm/bin/lli"
	} else if path, err := exec.LookPath("lli"); err == nil {
		// Ensure the found executable is not in the current directory (PATH=.)
		if !strings.Contains(path, "/") && !strings.Contains(path, "\\") {
			log.Fatalf("lli found but path is suspect: %s", path)
		}
		lli = path
	} else {
		log.Fatalf("lli (LLVM JIT) not found in path or standard homebrew locations")
	}
	
	cmd := exec.Command(lli, "-O3", llFile, dataFile)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		log.Fatalf("LLVM execution failed: %v", err)
	}
}

// main orchestrates the stream filter.
func main() {
	if len(os.Args) < 4 {
		log.Fatalf("usage: strujit <schema> <predicate> <file> [--json]")
	}
	isJson := len(os.Args) >= 5 && os.Args[4] == "--json"
	
	fields, sch := parseSchema(os.Args[1])
	fld, op, val := parsePred(os.Args[2], fields)
	llFile := buildIR(sch, fld, op, val, isJson)
	defer os.Remove(llFile) // Clean up temp file
	
	execute(llFile, os.Args[3])
}
