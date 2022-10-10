package internal

import (
	"testing"
)

var validCustomExamples = []string{
	"umh.v1.exampleEnterprise.exampleSite.exampleArea.exampleProductionLine.exampleWorkCell.custom.processValue.abc",
	"umh.v1.exampleEnterprise.exampleSite.exampleArea.exampleProductionLine.exampleWorkCell.custom.processValueString.abc",
	"umh.v1.exampleEnterprise.exampleSite.exampleArea.exampleProductionLine.exampleWorkCell.custom.processValue.abc",
	"umh.v1.exampleEnterprise.exampleSite.exampleArea.exampleProductionLine.exampleWorkCell.custom.processValue.abc.aawd_awdq2-dk-d-d-d.dd.desfi8dye_-",
	"umh.v1.exampleEnterprise.exampleSite.exampleArea.exampleProductionLine.exampleWorkCell.custom.processValueString.abc",
	"umh.v1.exampleEnterprise.exampleSite.exampleArea.exampleProductionLine.exampleWorkCell.custom.processValueString.abc.adw.adw_axxa1AA",
}

var validStandardExamples = []string{
	"umh.v1.exampleEnterprise.exampleSite.exampleArea.exampleProductionLine.exampleWorkCell.standard.job.add",
	"umh.v1.exampleEnterprise.exampleSite.exampleArea.exampleProductionLine.exampleWorkCell.standard.job.delete",
	"umh.v1.exampleEnterprise.exampleSite.exampleArea.exampleProductionLine.exampleWorkCell.standard.job.end",
	"umh.v1.exampleEnterprise.exampleSite.exampleArea.exampleProductionLine.exampleWorkCell.standard.shift.add",
	"umh.v1.exampleEnterprise.exampleSite.exampleArea.exampleProductionLine.exampleWorkCell.standard.shift.delete",
	"umh.v1.exampleEnterprise.exampleSite.exampleArea.exampleProductionLine.exampleWorkCell.standard.product-type.add",
	"umh.v1.exampleEnterprise.exampleSite.exampleArea.exampleProductionLine.exampleWorkCell.standard.product.add",
	"umh.v1.exampleEnterprise.exampleSite.exampleArea.exampleProductionLine.exampleWorkCell.standard.product.modify",
	"umh.v1.exampleEnterprise.exampleSite.exampleArea.exampleProductionLine.exampleWorkCell.standard.state.add",
	"umh.v1.exampleEnterprise.exampleSite.exampleArea.exampleProductionLine.exampleWorkCell.standard.state.overwrite",
	"umh.v1.exampleEnterprise.exampleSite.exampleArea.exampleProductionLine.exampleWorkCell.standard.state.activity",
	"umh.v1.exampleEnterprise.exampleSite.exampleArea.exampleProductionLine.exampleWorkCell.standard.state.reason",
}

var validRawExamples = []string{
	"umh.v1.exampleEnterprise.exampleSite.exampleArea.exampleProductionLine.exampleWorkCell.raw.raw",
	"umh.v1.exampleEnterprise.exampleSite.exampleArea.exampleProductionLine.exampleWorkCell.raw.raw",
	"umh.v1.exampleEnterprise.exampleSite.exampleArea.exampleProductionLine.exampleWorkCell.raw.rawImage.abc.00-B0-D0-63-C2-26",
}

var invalidExamples = []string{
	"umh.v1.exampleEnterprise.exampleSite.exampleArea.exampleProductionLine.exampleWorkCell.custom",
	"umh.v1.exampleEnterprise.exampleSite.exampleArea.exampleProductionLine.exampleWorkCell.custom.",
	"umh.v1.exampleEnterprise.exampleSite.exampleArea.exampleProductionLine.exampleWorkCell.abc.aa",
}

func TestIsKafkaTopicV1Valid(t *testing.T) {

	for _, example := range validCustomExamples {
		if !IsKafkaTopicV1Valid(example) {
			t.Errorf("Topic %s should be valid", example)
		}
	}
	for _, example := range validStandardExamples {
		if !IsKafkaTopicV1Valid(example) {
			t.Errorf("Topic %s should be valid", example)
		}
	}
	for _, example := range validRawExamples {
		if !IsKafkaTopicV1Valid(example) {
			t.Errorf("Topic %s should be valid", example)
		}
	}

	for _, example := range invalidExamples {
		if IsKafkaTopicV1Valid(example) {
			t.Errorf("Topic %s should be invalid", example)
		}
	}
}

func TestGetTopicInformationV1(t *testing.T) {
	for _, example := range validCustomExamples {
		ti, err := getTopicInformationV1(example)
		if err != nil {
			t.Errorf("Topic %s should be valid", example)
		}
		if ti.Display() != example {
			t.Errorf("Topic:\n\t%s\nshould have display\n\t%s\n", ti.Display(), example)
		}
	}
	for _, example := range validStandardExamples {
		ti, err := getTopicInformationV1(example)
		if err != nil {
			t.Errorf("Topic %s should be valid", example)
		}
		if ti.Display() != example {
			t.Errorf("Topic:\n\t%s\nshould have display\n\t%s\n", ti.Display(), example)
		}
	}

	for _, example := range validRawExamples {
		ti, err := getTopicInformationV1(example)
		if err != nil {
			t.Errorf("Topic %s should be valid", example)
		}
		if ti.Display() != example {
			t.Errorf("Topic:\n\t%s\nshould have display\n\t%s\n", ti.Display(), example)
		}
	}
}
