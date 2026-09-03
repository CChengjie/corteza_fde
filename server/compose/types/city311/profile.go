package city311

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// ProfileUpdate is the existing profile_update PATCH schema. Nil means omitted;
// an empty array explicitly clears a collection. JSON null is not a field value.
type ProfileUpdate struct {
	DisplayName       *string          `json:"display_name,omitempty"`
	PhoneNumbers      *[]PhoneNumber   `json:"phone_numbers,omitempty"`
	Addresses         *[]Address       `json:"addresses,omitempty"`
	PrimaryCategory   *ContactCategory `json:"primary_category,omitempty"`
	PreferredLanguage *Language        `json:"preferred_language,omitempty"`
}

func (input *ProfileUpdate) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return fmt.Errorf("profile update must be an object")
	}
	type plain ProfileUpdate
	var value plain
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return err
	}
	var fields any
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if profileContainsNull(fields) {
		return fmt.Errorf("profile fields must not be null")
	}
	*input = ProfileUpdate(value)
	return nil
}

func profileContainsNull(value any) bool {
	switch item := value.(type) {
	case nil:
		return true
	case map[string]any:
		for _, field := range item {
			if profileContainsNull(field) {
				return true
			}
		}
	case []any:
		for _, field := range item {
			if profileContainsNull(field) {
				return true
			}
		}
	}
	return false
}

type LanguagePreference struct {
	Language Language `json:"language"`
}
