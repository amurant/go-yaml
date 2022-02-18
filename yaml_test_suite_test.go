package yaml

import (
	"io/ioutil"
	"os"
	"path"
	"runtime"
	"strings"
	"testing"
)

func escaped(b []byte) string {
	str := ""
	for _, c := range string(b) {
		switch c {
		case '\\':
			str += "\\\\"
		case 0:
			str += "\\0"
		case '\b':
			str += "\\b"
		case '\n':
			str += "\\n"
		case '\r':
			str += "\\r"
		case '\t':
			str += "\\t"
		default:
			str += string(c)
		}
	}
	return str
}

func next_event(p *parser) *string {
	e := &p.event

	str := ""

	if p.event.typ == yaml_NO_EVENT {
		p.event = yaml_event_t{}

		// No events after the end of the stream or error.
		if p.parser.stream_end_produced || p.parser.error != yaml_NO_ERROR {
			return nil
		}

		// Generate the next event.
		if !yaml_parser_state_machine(&p.parser, &p.event) {
			return nil
		}
	}

	switch p.event.typ {
	case yaml_NO_EVENT:
		return nil

	case yaml_STREAM_START_EVENT:
		str = "+STR"
	case yaml_STREAM_END_EVENT:
		str = "-STR"
	case yaml_DOCUMENT_START_EVENT:
		if e.implicit {
			str = "+DOC"
		} else {
			str = "+DOC ---"
		}
	case yaml_DOCUMENT_END_EVENT:
		if e.implicit {
			str = "-DOC"
		} else {
			str = "-DOC ..."
		}
	case yaml_MAPPING_START_EVENT:
		if e.mapping_style() == yaml_FLOW_MAPPING_STYLE {
			str = "+MAP {}"
		} else {
			str = "+MAP"
		}
		if e.anchor != nil {
			str += " &" + string(e.anchor)
		}
		if e.tag != nil {
			str += " <" + string(e.tag) + ">"
		}
	case yaml_MAPPING_END_EVENT:
		str = "-MAP"
	case yaml_SEQUENCE_START_EVENT:
		if e.sequence_style() == yaml_FLOW_SEQUENCE_STYLE {
			str = "+SEQ []"
		} else {
			str = "+SEQ"
		}
		if e.anchor != nil {
			str += " &" + string(e.anchor)
		}
		if e.tag != nil {
			str += " <" + string(e.tag) + ">"
		}
	case yaml_SEQUENCE_END_EVENT:
		str = "-SEQ"
	case yaml_SCALAR_EVENT:
		str = "=VAL"
		if e.anchor != nil {
			str += " &" + string(e.anchor)
		}
		if e.tag != nil {
			str += " <" + string(e.tag) + ">"
		}
		style := e.scalar_style()
		switch {
		case style == yaml_PLAIN_SCALAR_STYLE:
			str += " :"
		case style == yaml_SINGLE_QUOTED_SCALAR_STYLE:
			str += " '"
		case style == yaml_DOUBLE_QUOTED_SCALAR_STYLE:
			str += " \""
		case style == yaml_LITERAL_SCALAR_STYLE:
			str += " |"
		case style == yaml_FOLDED_SCALAR_STYLE:
			str += " >"
		}
		str += escaped(e.value)
	case yaml_ALIAS_EVENT:
		str = "=ALI *" + string(e.anchor)
	default:
		panic("internal error: Unexpected event: (please report): " + p.event.typ.String())
	}

	yaml_event_delete(e)
	e.typ = yaml_NO_EVENT

	return &str
}

func reset_path() {
	_, filename, _, _ := runtime.Caller(0)
	dir := path.Join(path.Dir(filename), ".")
	err := os.Chdir(dir)
	if err != nil {
		panic(err)
	}
}

type TestCase struct {
	Name *string `yaml:"name,omitempty"`
	From *string `yaml:"from,omitempty"`
	Tags *string `yaml:"tags,omitempty"`
	Yaml string  `yaml:"yaml,omitempty"`
	Tree *string `yaml:"tree,omitempty"`
	Json *string `yaml:"json,omitempty"`
	Dump *string `yaml:"dump,omitempty"`
	Skip *bool   `yaml:"skip,omitempty"`
	Fail *bool   `yaml:"fail,omitempty"`
}

func fixSpecialChars(yaml string) string {
	yaml = strings.ReplaceAll(yaml, "————»", "\t")
	yaml = strings.ReplaceAll(yaml, "———»", "\t")
	yaml = strings.ReplaceAll(yaml, "——»", "\t")
	yaml = strings.ReplaceAll(yaml, "—»", "\t")
	yaml = strings.ReplaceAll(yaml, "»", "\t")
	yaml = strings.ReplaceAll(yaml, "␣", " ")
	yaml = strings.ReplaceAll(yaml, "↵", "\n")
	yaml = strings.ReplaceAll(yaml, "∎", "")
	yaml = strings.TrimSuffix(yaml, "\n")
	yaml = strings.TrimSuffix(yaml, "\n") + "\n"
	return yaml
}

func buildTree(test *TestCase) (string, bool) {
	yaml := fixSpecialChars(test.Yaml)
	full_result := ""
	indent := 0
	parser := newParser([]byte(yaml))
	parser.parser.lookahead = 0
	for {
		result := next_event(parser)
		if result == nil {
			break
		}

		if (*result)[0] == '-' {
			indent--
		}

		ok := strings.Repeat(" ", indent) + *result + "\n"

		full_result += ok

		if (*result)[0] == '+' {
			indent++
		}
	}

	return full_result, parser.parser.error != yaml_NO_ERROR
}

func listTokens(yaml string) string {
	yaml = fixSpecialChars(yaml)
	parser := newParser([]byte(yaml))

	tokens := ""
	for {
		token := peek_token(&parser.parser)

		if token != nil {
			tokens += token.typ.String() + "\n"
		}

		if token == nil || token.typ == yaml_STREAM_END_TOKEN {
			if len(parser.parser.problem) > 0 {
				tokens += parser.parser.problem + "\n"
			}

			break
		}

		skip_token(&parser.parser)
	}

	return tokens
}

const testDir = "./tests/yaml-test-suite/src/"

func testYAMLSuite(t *testing.T, name string) {
	f, err := os.Open(testDir + name)
	if err != nil {
		t.Error(err)
		return
	}

	value := []TestCase{}
	err = NewDecoder(f).Decode(&value)
	if err != nil {
		t.Error(err)
		return
	}

	t.Run(name, func(t *testing.T) {
		tree_val := ""
		skip := false
		fail := false
		for _, test := range value {
			if test.Skip != nil {
				skip = *test.Skip
			}
			if test.Tree != nil {
				tree_val = fixSpecialChars(*test.Tree)
			}
			fail = (test.Fail != nil) && *test.Fail || false

			if skip {
				continue
			}

			full_result, found_error := buildTree(&test)

			full_result = fixSpecialChars(full_result)

			t.Logf("yaml:\n%s", test.Yaml)

			t.Logf("tokens:\n%s", listTokens(test.Yaml))

			if fail && !found_error {
				t.Errorf("expected error, but found none")
			} else if !fail && found_error {
				t.Errorf("expected no error, but found one")
			} else if test.Tree != nil && full_result != tree_val {
				t.Errorf(""+
					"expected:\n%s\n"+
					"provided:\n%s", tree_val, full_result)
			}
		}
	})
}

func contains(s []string, e string) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}

func TestYAMLSuite(t *testing.T) {
	reset_path()

	failing := []string{
		"2CMS.yaml", "4H7K.yaml", "6LVF.yaml", "9JBA.yaml", "CVW2.yaml", "DK4H.yaml", "EW3V.yaml",
		"H7J7.yaml", "KS4U.yaml", "P2EQ.yaml", "S98Z.yaml", "SU74.yaml", "VJP3.yaml", "WZ62.yaml",
		"YJV2.yaml", "2LFX.yaml", "4JVG.yaml", "9C9N.yaml", "9KBC.yaml", "CXX2.yaml", "DK95.yaml",
		"FP8R.yaml", "HRE5.yaml", "M7A3.yaml", "QB6E.yaml", "SR86.yaml", "T833.yaml", "W4TN.yaml",
		"X4QW.yaml", "ZCZ6.yaml", "3HFZ.yaml", "5LLU.yaml", "9HCY.yaml", "C2SP.yaml", "DK3J.yaml",
		"EB22.yaml", "G5U8.yaml", "JEF9.yaml", "MUS6.yaml", "RHX7.yaml", "SU5Z.yaml", "U99R.yaml",
		"W9L4.yaml", "Y79Y.yaml", "ZXT5.yaml",
	}

	items, _ := ioutil.ReadDir(testDir)
	for _, item := range items {
		if item.IsDir() {
			continue
		}

		if contains(failing, item.Name()) {
			continue
		}

		testYAMLSuite(t, item.Name())
	}
}
