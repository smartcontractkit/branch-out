package config

import (
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			field := buildStructField(t, tc)
			flagSet := pflag.NewFlagSet("test", pflag.ContinueOnError)
			v := viper.New()

			err := processField(flagSet, v, field)
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
				tc.desc,
				flagSet.Lookup(tc.flag).Usage,
				"description should be set in flag set",
			)
		})
	}
}

func TestMustBind(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{}
	v := viper.New()

	require.NotPanics(t, func() {
		MustBindFlagsAndEnvs(cmd, v)
	}, "binding flags and envs should not panic")

	require.NotPanics(t, func() {
		MustBindFlags(cmd)
	}, "binding flags should not panic")

	require.NotPanics(t, func() {
		MustBindEnvs(v)
	}, "binding envs should not panic")
}

func TestBind(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{}
	v := viper.New()

	err := BindFlagsAndEnvs(cmd, v)
	require.NoError(t, err, "binding flags and envs should not return an error")

	err = BindFlags(cmd)
	require.NoError(t, err, "binding flags should not return an error")

	v = viper.New()
	err = BindEnvs(v)
	require.NoError(t, err, "binding envs should not return an error")
}
