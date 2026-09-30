package cli

import (
	"errors"
	"flag"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    Options
		wantErr bool
	}{
		{
			name: "no args defaults to current directory",
			args: nil,
			want: Options{Path: ".", Format: FormatText},
		},
		{
			name: "positional path",
			args: []string{"/var/log"},
			want: Options{Path: filepath.Clean("/var/log"), Format: FormatText},
		},
		{
			name: "path flag",
			args: []string{"--path", "/data"},
			want: Options{Path: filepath.Clean("/data"), Format: FormatText},
		},
		{
			name: "path flag takes precedence over positional",
			args: []string{"--path", "/data", "/ignored"},
			want: Options{Path: filepath.Clean("/data"), Format: FormatText},
		},
		{
			name: "equals form",
			args: []string{"--path=/data", "--format=json"},
			want: Options{Path: filepath.Clean("/data"), Format: FormatJSON},
		},
		{
			name: "json format",
			args: []string{"--format", "json"},
			want: Options{Path: ".", Format: FormatJSON},
		},
		{
			name: "verbose with output and positional",
			args: []string{"--verbose", "--output", "result.json", "/x"},
			want: Options{Path: filepath.Clean("/x"), Format: FormatText, Output: "result.json", Verbose: true},
		},
		{
			name: "version flag",
			args: []string{"--version"},
			want: Options{Path: ".", Format: FormatText, Version: true},
		},
		{
			name: "threads flag",
			args: []string{"--threads", "4"},
			want: Options{Path: ".", Format: FormatText, Threads: 4},
		},
		{
			name: "threads zero means auto",
			args: []string{"--threads", "0"},
			want: Options{Path: ".", Format: FormatText},
		},
		{
			name: "threads at maximum is accepted",
			args: []string{"--threads", "1024"},
			want: Options{Path: ".", Format: FormatText, Threads: 1024},
		},
		{
			name: "no-emoji flag",
			args: []string{"--no-emoji"},
			want: Options{Path: ".", Format: FormatText, NoEmoji: true},
		},
		{
			name: "plain flag",
			args: []string{"--plain"},
			want: Options{Path: ".", Format: FormatText, Plain: true},
		},
		{
			name:    "negative threads is rejected",
			args:    []string{"--threads", "-1"},
			wantErr: true,
		},
		{
			name:    "threads above maximum is rejected",
			args:    []string{"--threads", "99999"},
			wantErr: true,
		},
		{
			// JSON 报告没有排版，--plain 与之矛盾；静默忽略会让用户误以为生效。
			name:    "plain with json is rejected",
			args:    []string{"--plain", "--format", "json"},
			wantErr: true,
		},
		{
			name: "path is cleaned",
			args: []string{"./a/../b"},
			want: Options{Path: "b", Format: FormatText},
		},
		{
			name: "empty positional falls back to current directory",
			args: []string{""},
			want: Options{Path: ".", Format: FormatText},
		},
		{
			name:    "invalid format",
			args:    []string{"--format", "xml"},
			wantErr: true,
		},
		{
			name:    "too many positionals",
			args:    []string{"a", "b"},
			wantErr: true,
		},
		{
			name:    "unknown flag",
			args:    []string{"--nope"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Parse(%v) 期望错误，实际返回 %+v", tt.args, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse(%v) 返回意外错误: %v", tt.args, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Parse(%v) = %+v, want %+v", tt.args, got, tt.want)
			}
		})
	}
}

// TestParse_Help 验证帮助请求以 flag.ErrHelp 返回，由调用方决定退出码。
func TestParse_Help(t *testing.T) {
	for _, arg := range []string{"-h", "--help", "-help"} {
		t.Run(arg, func(t *testing.T) {
			if _, err := Parse([]string{arg}); !errors.Is(err, flag.ErrHelp) {
				t.Errorf("Parse(%q) 错误 = %v, want flag.ErrHelp", arg, err)
			}
		})
	}
}

func TestUsage_DocumentsSemantics(t *testing.T) {
	usage := Usage()

	// 统计口径必须在帮助中显式声明：同类工具最容易产生争议的地方正是
	// 「逻辑大小 vs 磁盘占用」「是否跟随符号链接」「是否去重硬链接」。
	mustContain := []string{
		"--path", "--format", "--output", "--threads", "--verbose",
		"--no-emoji", "--plain", "--version",
		"退出码", "统计口径", "st_size", "符号链接", "硬链接",
	}
	for _, want := range mustContain {
		if !strings.Contains(usage, want) {
			t.Errorf("用法说明缺少 %q", want)
		}
	}
}
