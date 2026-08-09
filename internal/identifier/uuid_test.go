package identifier

import "testing"

func TestIsUUID(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "lowercase", value: "11111111-1111-4111-8111-111111111111", want: true},
		{name: "uppercase", value: "AAAAAAAA-BBBB-4CCC-8DDD-EEEEEEEEEEEE", want: true},
		{name: "surrounding whitespace", value: " 11111111-1111-4111-8111-111111111111 ", want: true},
		{name: "empty", value: "", want: false},
		{name: "braced", value: "{11111111-1111-4111-8111-111111111111}", want: false},
		{name: "urn", value: "urn:uuid:11111111-1111-4111-8111-111111111111", want: false},
		{name: "wrong hyphen", value: "111111111-111-4111-8111-111111111111", want: false},
		{name: "non hex", value: "11111111-1111-4111-8111-11111111111z", want: false},
		{name: "short", value: "11111111-1111-4111-8111-11111111111", want: false},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := IsUUID(test.value); got != test.want {
				t.Fatalf("IsUUID(%q) = %v, want %v", test.value, got, test.want)
			}
		})
	}
}
