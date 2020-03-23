package workspace

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-k8s/operator/pkg/apis/app/v1alpha1"
	"github.com/hashicorp/terraform/states/statefile"
	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"
)

// GetStateVersionDownloadURL retrieves download URL for state file
func (t *TerraformCloudClient) GetStateVersionDownloadURL(workspaceID string) (string, error) {
	stateVersion, err := t.Client.StateVersions.Current(context.TODO(), workspaceID)
	if err != nil {
		return "", fmt.Errorf("could not get current state version, WorkspaceID, %s, Error, %v", workspaceID, err)
	}
	return stateVersion.DownloadURL, nil
}

func convertValueToString(val cty.Value) string {
	ty := val.Type()
	switch {
	case ty.IsPrimitiveType():
		switch ty {
		case cty.String:
			{
				// Special behavior for JSON strings containing array or object
				src := []byte(val.AsString())
				ty, err := ctyjson.ImpliedType(src)
				// check for the special case of "null", which decodes to nil,
				// and just allow it to be printed out directly
				if err == nil && !ty.IsPrimitiveType() && strings.TrimSpace(val.AsString()) != "null" {
					jv, err := ctyjson.Unmarshal(src, ty)
					if err == nil {
						return jv.AsString()
					}
				}
			}
			return `"` + val.AsString() + `"`
		case cty.Bool:
			if val.True() {
				return "true"
			}
			return "false"
		case cty.Number:
			bf := val.AsBigFloat()
			return bf.Text('f', -1)
		default:
			return fmt.Sprintf("%#v", val)
		}
	case ty.IsListType() || ty.IsSetType() || ty.IsTupleType():
		var b bytes.Buffer
		b.WriteString("[")
		i := 0
		for it := val.ElementIterator(); it.Next(); {
			_, value := it.Element()
			b.WriteString(convertValueToString(value))
			if i < (val.LengthInt() - 1) {
				b.WriteString(",")
			}
			i++
		}
		b.WriteString("]")
		return b.String()
	case ty.IsMapType():
		var b bytes.Buffer
		b.WriteString("{")

		i := 0
		for it := val.ElementIterator(); it.Next(); {
			key, value := it.Element()
			b.WriteString(convertValueToString(key))
			b.WriteString(":")
			b.WriteString(convertValueToString(value))
			if i < (val.LengthInt() - 1) {
				b.WriteString(",")
			}
			i++
		}

		b.WriteString("}")
		return b.String()
	case ty.IsObjectType():
		var b bytes.Buffer
		b.WriteString("{")
		atys := ty.AttributeTypes()
		i := 0
		for attr := range atys {
			b.WriteString(`"`)
			b.WriteString(attr)
			b.WriteString(`"`)
			b.WriteString(":")
			val := val.GetAttr(attr)
			b.WriteString(convertValueToString(val))
			if i < (len(atys) - 1) {
				b.WriteString(",")
			}
			i++
		}
		b.WriteString("}")
		return b.String()
	}
	return ""
}

// GetOutputsFromState gets list of outputs from state file
func (t *TerraformCloudClient) GetOutputsFromState(stateDownloadURL string) ([]*v1alpha1.OutputStatus, error) {
	if stateDownloadURL == "" {
		return nil, fmt.Errorf("could not download blank state")
	}
	data, err := t.Client.StateVersions.Download(context.TODO(), stateDownloadURL)
	if err != nil {
		return nil, fmt.Errorf("could not download state, Error, %v", err)
	}
	reader := bytes.NewReader(data)
	file, err := statefile.Read(reader)
	if err != nil {
		return nil, fmt.Errorf("could not read state file, Error, %v", err)
	}
	outputValues := file.State.Modules[""].OutputValues
	outputs := []*v1alpha1.OutputStatus{}
	for key, value := range outputValues {
		if !value.Sensitive {
			if err != nil {
				return outputs, fmt.Errorf("output value could not be converted to string, Error, %v", err)
			}
			outputs = append(outputs, &v1alpha1.OutputStatus{Key: key, Value: convertValueToString(value.Value)})
		}
	}
	return outputs, nil
}

// CheckOutputs retrieves outputs for a run.
func (t *TerraformCloudClient) CheckOutputs(workspaceID string, runID string) ([]*v1alpha1.OutputStatus, error) {
	outputs := []*v1alpha1.OutputStatus{}
	if runID == "" {
		return outputs, nil
	}
	stateDownloadURL, err := t.GetStateVersionDownloadURL(workspaceID)
	if err != nil {
		return outputs, err
	}

	outputs, err = t.GetOutputsFromState(stateDownloadURL)
	if err != nil {
		return outputs, err
	}

	return outputs, nil
}
