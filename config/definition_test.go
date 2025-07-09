package config

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBindField(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		field         Field
		expectedError error
	}{
		{
			name: "string field",
			field: Field{
				EnvVar:      "TEST_STRING_FIELD",
				Description: "A string field",
				Example:     "test-string-field",
				Flag:        "test-string-field",
				ShortFlag:   "s",
				Type:        reflect.TypeOf(""),
				Default:     "default-value",
			},
		},
		{
			name: "int field",
			field: Field{
				EnvVar:      "TEST_INT_FIELD",
				Description: "An int field",
				Example:     4,
				Flag:        "test-int-field",
				ShortFlag:   "i",
				Type:        reflect.TypeOf(0),
				Default:     42,
			},
		},
		{
			name: "bool field",
			field: Field{
				EnvVar:      "TEST_BOOL_FIELD",
				Description: "A bool field",
				Type:        reflect.TypeOf(false),
				Example:     true,
				Flag:        "test-bool-field",
			},
		},
		{
			name: "persistent field",
			field: Field{
				EnvVar:      "TEST_PERSISTENT_FIELD",
				Description: "A persistent field",
				Type:        reflect.TypeOf(""),
				Example:     "test-persistent-field",
				Flag:        "test-persistent-field",
				Persistent:  true,
			},
		},
		{
			name: "type mismatch",
			field: Field{
				EnvVar:      "TEST_TYPE_MISMATCH_FIELD",
				Description: "A type mismatch field",
				Example:     "test-type-mismatch-field",
				Flag:        "test-type-mismatch-field",
				Type:        reflect.TypeOf(0),
				Default:     "42",
			},
			expectedError: fmt.Errorf(
				ErrMsgTypeMismatch,
				"test-type-mismatch-field",
				reflect.TypeOf(0),
				reflect.TypeOf("42"),
				reflect.TypeOf("test-type-mismatch-field"),
			),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cmd := &cobra.Command{}
			v := viper.New()

			err := bindField(cmd, v, tc.field)
			if tc.expectedError != nil {
				require.Error(t, err, "expected an error from this test case")
				require.Equal(t, tc.expectedError, err, "expected error should match")
				return
			}
			require.NoError(t, err, "bindField should not return an error")

			if tc.field.Default != nil {
				assert.Equal(t, tc.field.Default, v.Get(tc.field.EnvVar), "default value should be set in viper")
			}

			flagSet := cmd.Flags()
			if tc.field.Persistent {
				flagSet = cmd.PersistentFlags()
			}

			flag := flagSet.Lookup(tc.field.Flag)
			require.NotNil(t, flag, "flag should be set")
			assert.Equal(t, tc.field.Flag, flag.Name, "flag name should match")
		})
	}
}
