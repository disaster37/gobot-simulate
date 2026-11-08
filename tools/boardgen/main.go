// Command boardgen generates board profiles from the pinned gobot module
// source. Run from the module root: go run ./tools/boardgen
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/disaster37/gobot-simulate/boardprofile"
)

var platformDirs = []string{
	"platforms/raspi",
	"platforms/beagleboard/beaglebone",
	"platforms/beagleboard/pocketbeagle",
	"platforms/asus/tinkerboard",
	"platforms/asus/tinkerboard2",
	"platforms/jetson",
	"platforms/chip",
	"platforms/orangepi/orangepi5pro",
	"platforms/radxa/rockpi",
	"platforms/radxa/zero",
	"platforms/pine64/rock64",
	"platforms/upboard/up2",
	"platforms/friendlyelec/nanopct6",
	"platforms/friendlyelec/nanopi",
	"platforms/dragonboard",
	"platforms/intel-iot/curie",
	"platforms/intel-iot/edison",
	"platforms/intel-iot/joule",
	"platforms/holystone/hs200",
}

func main() {
	var (
		outPath  = flag.String("out", "boardprofile/profiles_gen.go", "generated Go file")
		jsonPath = flag.String("json", "boardprofile/profiles_gen.json", "generated JSON catalog")
	)
	flag.Parse()

	gobotDir, err := gobotModuleDir()
	if err != nil {
		fail(err)
	}
	profiles, err := collect(gobotDir)
	if err != nil {
		fail(err)
	}
	profiles = append(profiles, pseudoProfiles()...)
	sort.Slice(profiles, func(i, j int) bool { return profiles[i].ID < profiles[j].ID })

	if err := writeGo(*outPath, profiles); err != nil {
		fail(err)
	}
	if err := writeJSON(*jsonPath, profiles); err != nil {
		fail(err)
	}
	fmt.Printf("boardgen: wrote %d profiles\n", len(profiles))
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "boardgen: %v\n", err)
	os.Exit(1)
}

func gobotModuleDir() (string, error) {
	data, err := os.ReadFile("go.mod")
	if err != nil {
		return "", err
	}
	re := regexp.MustCompile(`gobot\.io/x/gobot/v2 v([0-9][^\s]*)`)
	match := re.FindSubmatch(data)
	if match == nil {
		return "", fmt.Errorf("gobot.io/x/gobot/v2 not found in go.mod")
	}
	root := os.Getenv("GOMODCACHE")
	if root == "" {
		out, err := exec.CommandContext(context.Background(), "go", "env", "GOMODCACHE").Output()
		if err != nil {
			return "", fmt.Errorf("go env GOMODCACHE: %w", err)
		}
		root = strings.TrimSpace(string(out))
	}
	return filepath.Join(root, "gobot.io", "x", "gobot", "v2@v"+string(match[1])), nil
}

type accumulator struct {
	pins   map[string]struct{}
	analog map[string]struct{}
	pwm    map[string]struct{}
	i2c    map[int]struct{}
	spi    map[int]struct{}
	defI2C *int
	defSPI *int
}

func newAccumulator() *accumulator {
	return &accumulator{
		pins:   make(map[string]struct{}),
		analog: make(map[string]struct{}),
		pwm:    make(map[string]struct{}),
		i2c:    make(map[int]struct{}),
		spi:    make(map[int]struct{}),
	}
}

func collect(gobotDir string) ([]boardprofile.Profile, error) {
	var out []boardprofile.Profile
	for _, rel := range platformDirs {
		dir := filepath.Join(gobotDir, rel)
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", rel, err)
		}
		acc := newAccumulator()
		fset := token.NewFileSet()
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			file, err := parser.ParseFile(fset, filepath.Join(dir, entry.Name()), nil, 0)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", entry.Name(), err)
			}
			scanFile(file, acc)
		}
		base := filepath.Base(rel)
		platform := strings.TrimPrefix(strings.ReplaceAll(rel, "/", "-"), "platforms-")
		profile := boardprofile.Profile{
			ID:            base,
			Name:          title(base),
			Platform:      platform,
			DigitalPins:   pinList(acc.pins, "digital"),
			AnalogPins:    pinList(acc.analog, "analog"),
			PWMPins:       pinList(acc.pwm, "pwm"),
			DefaultI2CBus: deref(acc.defI2C, 1),
			DefaultSPIBus: deref(acc.defSPI, 0),
			Permissive:    len(acc.pins) == 0 && len(acc.analog) == 0 && len(acc.pwm) == 0,
		}
		for number := range acc.i2c {
			profile.I2CBuses = append(profile.I2CBuses, boardprofile.Bus{Transport: "i2c", Number: number})
		}
		for number := range acc.spi {
			profile.SPIBuses = append(profile.SPIBuses, boardprofile.Bus{Transport: "spi", Number: number})
		}
		sort.Slice(profile.I2CBuses, func(i, j int) bool { return profile.I2CBuses[i].Number < profile.I2CBuses[j].Number })
		sort.Slice(profile.SPIBuses, func(i, j int) bool { return profile.SPIBuses[i].Number < profile.SPIBuses[j].Number })
		if len(profile.I2CBuses) == 0 {
			profile.I2CBuses = []boardprofile.Bus{{Transport: "i2c", Number: profile.DefaultI2CBus}}
		}
		if len(profile.SPIBuses) == 0 {
			profile.SPIBuses = []boardprofile.Bus{{Transport: "spi", Number: profile.DefaultSPIBus}}
		}
		out = append(out, profile)
	}
	return out, nil
}

func scanFile(file *ast.File, acc *accumulator) {
	ast.Inspect(file, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.GenDecl:
			if n.Tok == token.CONST {
				scanConsts(n, acc)
			}
		case *ast.ValueSpec:
			for _, name := range n.Names {
				scanValue(name.Name, n, acc)
			}
		case *ast.AssignStmt:
			scanAssign(n, acc)
		}
		return true
	})
}

func scanConsts(decl *ast.GenDecl, acc *accumulator) {
	for _, spec := range decl.Specs {
		value, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		for i, name := range value.Names {
			if i >= len(value.Values) {
				continue
			}
			lit, ok := value.Values[i].(*ast.BasicLit)
			if !ok || lit.Kind != token.INT {
				continue
			}
			number, err := strconv.Atoi(lit.Value)
			if err != nil {
				continue
			}
			lower := strings.ToLower(name.Name)
			switch {
			case strings.Contains(lower, "defaulti2cbus"):
				v := number
				acc.defI2C = &v
				acc.i2c[number] = struct{}{}
			case strings.Contains(lower, "defaultspibus"):
				v := number
				acc.defSPI = &v
				acc.spi[number] = struct{}{}
			}
		}
	}
}

func scanValue(name string, spec *ast.ValueSpec, acc *accumulator) {
	lower := strings.ToLower(name)
	if !strings.Contains(lower, "pin") && !strings.Contains(lower, "pwm") && !strings.Contains(lower, "gpio") {
		return
	}
	for _, value := range spec.Values {
		keys := mapKeys(value)
		if len(keys) == 0 {
			continue
		}
		switch {
		case strings.Contains(lower, "analog"):
			for _, key := range keys {
				if isPlausiblePin(key) {
					acc.analog[key] = struct{}{}
				}
			}
		case strings.Contains(lower, "pwm"):
			for _, key := range keys {
				if !isPlausiblePin(key) {
					continue
				}
				acc.pwm[key] = struct{}{}
				if isNumericPin(key) {
					acc.pins[key] = struct{}{}
				}
			}
		default:
			for _, key := range keys {
				if isPlausiblePin(key) {
					acc.pins[key] = struct{}{}
				}
			}
		}
	}
}

func mapKeys(node ast.Node) []string {
	lit, ok := node.(*ast.CompositeLit)
	if !ok {
		return nil
	}
	var keys []string
	for _, element := range lit.Elts {
		kv, ok := element.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.BasicLit)
		if !ok || key.Kind != token.STRING {
			continue
		}
		unquoted, err := strconv.Unquote(key.Value)
		if err != nil || unquoted == "" {
			continue
		}
		keys = append(keys, unquoted)
	}
	return keys
}

// scanAssign extracts bus-number validators from assignments such as
// "i2cBusNumberValidator := adaptors.NewBusNumberValidator([]int{0, 1})".
// The variable name distinguishes I2C from SPI regardless of construction order.
func scanAssign(stmt *ast.AssignStmt, acc *accumulator) {
	for i, lhs := range stmt.Lhs {
		if i >= len(stmt.Rhs) {
			continue
		}
		ident, ok := lhs.(*ast.Ident)
		if !ok {
			continue
		}
		call, ok := stmt.Rhs[i].(*ast.CallExpr)
		if !ok {
			continue
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "NewBusNumberValidator" || len(call.Args) == 0 {
			continue
		}
		numbers := intSlice(call.Args[0])
		if len(numbers) == 0 {
			continue
		}
		lower := strings.ToLower(ident.Name)
		switch {
		case strings.Contains(lower, "i2c"):
			for _, number := range numbers {
				acc.i2c[number] = struct{}{}
			}
		case strings.Contains(lower, "spi"):
			for _, number := range numbers {
				acc.spi[number] = struct{}{}
			}
		}
	}
}

func intSlice(node ast.Node) []int {
	lit, ok := node.(*ast.CompositeLit)
	if !ok {
		return nil
	}
	numbers := make([]int, 0, len(lit.Elts))
	for _, element := range lit.Elts {
		basic, ok := element.(*ast.BasicLit)
		if !ok || basic.Kind != token.INT {
			continue
		}
		number, err := strconv.Atoi(basic.Value)
		if err != nil {
			continue
		}
		numbers = append(numbers, number)
	}
	return numbers
}

func isPlausiblePin(value string) bool {
	lower := strings.ToLower(value)
	if lower == "" {
		return false
	}
	return !strings.Contains(lower, "thermal") &&
		!strings.Contains(lower, "zone") &&
		!strings.Contains(lower, "path")
}

func pinList(values map[string]struct{}, mode string) []boardprofile.Pin {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return lessPin(keys[i], keys[j]) })
	out := make([]boardprofile.Pin, 0, len(keys))
	for _, key := range keys {
		out = append(out, boardprofile.Pin{ID: key, Label: key, Mode: mode})
	}
	return out
}

func lessPin(a, b string) bool {
	ai, aerr := strconv.Atoi(a)
	bi, berr := strconv.Atoi(b)
	if aerr == nil && berr == nil {
		return ai < bi
	}
	if aerr == nil {
		return true
	}
	if berr == nil {
		return false
	}
	return a < b
}

func isNumericPin(value string) bool {
	_, err := strconv.Atoi(value)
	return err == nil
}

func deref(value *int, fallback int) int {
	if value == nil {
		return fallback
	}
	return *value
}

func title(value string) string {
	if value == "" {
		return ""
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

func pseudoProfiles() []boardprofile.Profile {
	return []boardprofile.Profile{
		{ID: "arest", Name: "aREST", Platform: "arest", Permissive: true, DefaultI2CBus: 0, DefaultSPIBus: 0},
		{ID: "firmata", Name: "Firmata", Platform: "firmata", Permissive: true, DefaultI2CBus: 0, DefaultSPIBus: 0},
		{ID: "particle", Name: "Particle", Platform: "particle", Permissive: true, DefaultI2CBus: 0, DefaultSPIBus: 0},
		{ID: "simulated", Name: "Simulated", Platform: "simulated", Permissive: true, DefaultI2CBus: 0, DefaultSPIBus: 0},
	}
}

func writeGo(path string, profiles []boardprofile.Profile) error {
	var buf bytes.Buffer
	buf.WriteString("// Code generated by tools/boardgen; DO NOT EDIT.\n\n")
	buf.WriteString("package boardprofile\n\n")
	buf.WriteString("var generatedProfiles = []Profile{\n")
	for _, profile := range profiles {
		buf.WriteString("\t{\n")
		fmt.Fprintf(&buf, "\t\tID: %q,\n", profile.ID)
		fmt.Fprintf(&buf, "\t\tName: %q,\n", profile.Name)
		fmt.Fprintf(&buf, "\t\tPlatform: %q,\n", profile.Platform)
		writePins(&buf, "DigitalPins", profile.DigitalPins)
		writePins(&buf, "AnalogPins", profile.AnalogPins)
		writePins(&buf, "PWMPins", profile.PWMPins)
		writeBuses(&buf, "I2CBuses", profile.I2CBuses)
		writeBuses(&buf, "SPIBuses", profile.SPIBuses)
		fmt.Fprintf(&buf, "\t\tDefaultI2CBus: %d,\n", profile.DefaultI2CBus)
		fmt.Fprintf(&buf, "\t\tDefaultSPIBus: %d,\n", profile.DefaultSPIBus)
		if profile.Permissive {
			buf.WriteString("\t\tPermissive: true,\n")
		}
		buf.WriteString("\t},\n")
	}
	buf.WriteString("}\n")
	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return err
	}
	return os.WriteFile(path, formatted, 0o600)
}

func writePins(buf *bytes.Buffer, name string, pins []boardprofile.Pin) {
	if len(pins) == 0 {
		return
	}
	fmt.Fprintf(buf, "\t\t%s: []Pin{\n", name)
	for _, pin := range pins {
		fmt.Fprintf(buf, "\t\t\t{ID: %q, Label: %q, Mode: %q},\n", pin.ID, pin.Label, pin.Mode)
	}
	buf.WriteString("\t\t},\n")
}

func writeBuses(buf *bytes.Buffer, name string, buses []boardprofile.Bus) {
	if len(buses) == 0 {
		return
	}
	fmt.Fprintf(buf, "\t\t%s: []Bus{\n", name)
	for _, bus := range buses {
		fmt.Fprintf(buf, "\t\t\t{Transport: %q, Number: %d},\n", bus.Transport, bus.Number)
	}
	buf.WriteString("\t\t},\n")
}

func writeJSON(path string, profiles []boardprofile.Profile) error {
	data, err := json.MarshalIndent(profiles, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o600)
}
