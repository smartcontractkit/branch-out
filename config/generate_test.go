package config

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessStruct(t *testing.T) {
	t.Parallel()

	type testStruct struct {
		Field1 string `mapstructure:"FIELD1"  desc:"A string field" flag:"field1" shortflag:"f" default:"default_value"`
		Field2 int    `mapstructure:"FIELD2"  desc:"An int field"   flag:"field2" shortflag:"f" default:"42"`
		Field3 bool   `mapstructure:"FIELD3"  desc:"A bool field"   flag:"field3" shortflag:"f" default:"true"`
		Nested struct {
			Field4 string `mapstructure:"FIELD4" desc:"A string field" flag:"field4" shortflag:"f" default:"default_value"`
		} `mapstructure:",squash"`
	}

	cmd := &cobra.Command{}
	v := viper.New()

	err := processStruct(reflect.TypeOf(testStruct{}), cmd, v)
	require.NoError(t, err, "error processing struct")

	assert.Equal(t, "default_value", v.GetString("FIELD1"), "default value should be set in viper")
	assert.Equal(t, 42, v.GetInt("FIELD2"), "default value should be set in viper")
	assert.True(t, v.GetBool("FIELD3"), "default value should be set in viper")
	assert.Equal(t, "default_value", v.GetString("FIELD4"), "default value should be set in viper")
}

func TestProcessField(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name         string
		valueType    reflect.Type
		mapstructure string
		desc         string
		flag         string
		shortflag    string
		defaultValue string
		expectError  bool
	}

	// helper function to build a struct field with the given tags
	buildStructField := func(
		t *testing.T,
		tc testCase,
	) reflect.StructField {
		t.Helper()
		require.NotNil(t, tc.valueType, "value type is required")

		tag := strings.Builder{}
		if tc.mapstructure != "" {
			tag.WriteString(`mapstructure:"`)
			tag.WriteString(tc.mapstructure)
			tag.WriteString(`" `)
		}
		if tc.desc != "" {
			tag.WriteString(`desc:"`)
			tag.WriteString(tc.desc)
			tag.WriteString(`" `)
		}
		if tc.flag != "" {
			tag.WriteString(`flag:"`)
			tag.WriteString(tc.flag)
			tag.WriteString(`" `)
		}
		if tc.shortflag != "" {
			tag.WriteString(`shortflag:"`)
			tag.WriteString(tc.shortflag)
			tag.WriteString(`" `)
		}
		if tc.defaultValue != "" {
			tag.WriteString(`default:"`)
			tag.WriteString(tc.defaultValue)
			tag.WriteString(`" `)
		}
		return reflect.StructField{
			Type: tc.valueType,
			Tag:  reflect.StructTag(tag.String()),
		}
	}

	testCases := []testCase{
		{
			name:         "string field",
			valueType:    reflect.TypeOf(""),
			mapstructure: "STRING_FIELD",
			desc:         "A string field",
			flag:         "string-field",
			shortflag:    "s",
			defaultValue: "default_value",
		},
		{
			name:         "int field",
			valueType:    reflect.TypeOf(0),
			mapstructure: "INT_FIELD",
			desc:         "An int field",
			flag:         "int-field",
			shortflag:    "i",
			defaultValue: "42",
		},
		{
			name:         "bool field",
			valueType:    reflect.TypeOf(false),
			mapstructure: "BOOL_FIELD",
			desc:         "A bool field",
			flag:         "bool-field",
			shortflag:    "b",
			defaultValue: "true",
		},
		{
			name:         "no flag field",
			valueType:    reflect.TypeOf(""),
			mapstructure: "NO_FLAG_FIELD",
			defaultValue: "default_value",
			expectError:  true,
		},
		{
			name:         "no desc field",
			valueType:    reflect.TypeOf(""),
			mapstructure: "NO_DESC_FIELD",
			flag:         "no-desc-field",
			defaultValue: "default_value",
			expectError:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			field := buildStructField(t, tc)
			flagSet := pflag.NewFlagSet("test", pflag.ContinueOnError)
			v := viper.New()

			err := processField(flagSet, v, field)
			if tc.expectError {
				require.Error(t, err, "expected error processing field")
				return
			}
			require.NoError(t, err, "error processing field")

			assert.Equal(t, tc.defaultValue, v.GetString(tc.mapstructure), "default value should be set in viper")
			assert.Equal(
				t,
				tc.defaultValue,
				flagSet.Lookup(tc.flag).DefValue,
				"default value should be set in flag set",
			)
			assert.Equal(
				t,
				fmt.Sprintf("%s (env: %s)", tc.desc, tc.mapstructure),
				flagSet.Lookup(tc.flag).Usage,
				"description should be set in flag set",
			)
		})
	}
}

func TestMustBindFlagsAndEnvs(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{}
	v := viper.New()

	require.NotPanics(t, func() {
		MustBindFlagsAndEnvs(cmd, v)
	}, "binding flags and envs should not panic")
}

func TestBindFlagsAndEnvs(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{}
	v := viper.New()

	err := BindFlagsAndEnvs(cmd, v)
	require.NoError(t, err, "binding flags and envs should not return an error")

	require.True(t, cmd.HasPersistentFlags(), "command should have persistent flags")
}
