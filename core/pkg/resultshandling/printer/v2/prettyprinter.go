package v2

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/armosec/k8s-interface/workloadinterface"
	"github.com/armosec/kubescape/core/cautils"
	"github.com/armosec/kubescape/core/pkg/resultshandling/printer"
	"github.com/armosec/opa-utils/objectsenvelopes"
	"github.com/armosec/opa-utils/reporthandling/apis"
	"github.com/armosec/opa-utils/reporthandling/results/v1/reportsummary"
	"github.com/enescakir/emoji"
	"github.com/olekukonko/tablewriter"
)

type PrettyPrinter struct {
	formatVersion      string
	writer             *os.File
	verboseMode        bool
	sortedControlNames []string
}

func NewPrettyPrinter(verboseMode bool, formatVersion string) *PrettyPrinter {
	return &PrettyPrinter{
		verboseMode:   verboseMode,
		formatVersion: formatVersion,
	}
}

func (prettyPrinter *PrettyPrinter) ActionPrint(opaSessionObj *cautils.OPASessionObj) {
	prettyPrinter.sortedControlNames = getSortedControlsNames(opaSessionObj.Report.SummaryDetails.Controls) // ListControls().All())

	if prettyPrinter.formatVersion == "v1" {
		prettyPrinter.printResults(&opaSessionObj.Report.SummaryDetails.Controls, opaSessionObj.AllResources)
	} else if prettyPrinter.formatVersion == "v2" {
		prettyPrinter.resourceTable(opaSessionObj.ResourcesResult, opaSessionObj.AllResources)
	}
	prettyPrinter.printSummaryTable(&opaSessionObj.Report.SummaryDetails)

}

func (prettyPrinter *PrettyPrinter) SetWriter(outputFile string) {
	prettyPrinter.writer = printer.GetWriter(outputFile)
}

func (prettyPrinter *PrettyPrinter) Score(score float32) {
}

func (prettyPrinter *PrettyPrinter) printResults(controls *reportsummary.ControlSummaries, allResources map[string]workloadinterface.IMetadata) {
	for i := 0; i < len(prettyPrinter.sortedControlNames); i++ {

		controlSummary := controls.GetControl(reportsummary.EControlCriteriaName, prettyPrinter.sortedControlNames[i]) //  summaryDetails.Controls ListControls().All() Controls.GetControl(ca)
		prettyPrinter.printTitle(controlSummary)
		prettyPrinter.printResources(controlSummary, allResources)
		prettyPrinter.printSummary(prettyPrinter.sortedControlNames[i], controlSummary)
	}
}

func (prettyPrinter *PrettyPrinter) printSummary(controlName string, controlSummary reportsummary.IControlSummary) {
	if controlSummary.GetStatus().IsSkipped() {
		return
	}
	cautils.SimpleDisplay(prettyPrinter.writer, "Summary - ")
	cautils.SuccessDisplay(prettyPrinter.writer, "Passed:%v   ", controlSummary.NumberOfResources().Passed())
	cautils.WarningDisplay(prettyPrinter.writer, "Excluded:%v   ", controlSummary.NumberOfResources().Excluded())
	cautils.FailureDisplay(prettyPrinter.writer, "Failed:%v   ", controlSummary.NumberOfResources().Failed())
	cautils.InfoDisplay(prettyPrinter.writer, "Total:%v\n", controlSummary.NumberOfResources().All())
	if controlSummary.GetStatus().IsFailed() {
		cautils.DescriptionDisplay(prettyPrinter.writer, "Remediation: %v\n", controlSummary.GetRemediation())
	}
	cautils.DescriptionDisplay(prettyPrinter.writer, "\n")

}
func (prettyPrinter *PrettyPrinter) printTitle(controlSummary reportsummary.IControlSummary) {
	cautils.InfoDisplay(prettyPrinter.writer, "[control: %s - %s] ", controlSummary.GetName(), getControlLink(controlSummary.GetID()))
	switch controlSummary.GetStatus().Status() {
	case apis.StatusSkipped:
		cautils.InfoDisplay(prettyPrinter.writer, "skipped %v\n", emoji.ConfusedFace)
	case apis.StatusFailed:
		cautils.FailureDisplay(prettyPrinter.writer, "failed %v\n", emoji.SadButRelievedFace)
	case apis.StatusExcluded:
		cautils.WarningDisplay(prettyPrinter.writer, "excluded %v\n", emoji.NeutralFace)
	case apis.StatusIrrelevant:
		cautils.SuccessDisplay(prettyPrinter.writer, "irrelevant %v\n", emoji.ConfusedFace)
	case apis.StatusError:
		cautils.WarningDisplay(prettyPrinter.writer, "error %v\n", emoji.ConfusedFace)
	default:
		cautils.SuccessDisplay(prettyPrinter.writer, "passed %v\n", emoji.ThumbsUp)
	}
	cautils.DescriptionDisplay(prettyPrinter.writer, "Description: %s\n", controlSummary.GetDescription())
	if controlSummary.GetStatus().Info() != "" {
		cautils.WarningDisplay(prettyPrinter.writer, "Reason: %v\n", controlSummary.GetStatus().Info())
	}
}
func (prettyPrinter *PrettyPrinter) printResources(controlSummary reportsummary.IControlSummary, allResources map[string]workloadinterface.IMetadata) {

	workloadsSummary := listResultSummary(controlSummary, allResources)

	failedWorkloads := groupByNamespaceOrKind(workloadsSummary, workloadSummaryFailed)
	excludedWorkloads := groupByNamespaceOrKind(workloadsSummary, workloadSummaryExclude)

	var passedWorkloads map[string][]WorkloadSummary
	if prettyPrinter.verboseMode {
		passedWorkloads = groupByNamespaceOrKind(workloadsSummary, workloadSummaryPassed)
	}
	if len(failedWorkloads) > 0 {
		cautils.FailureDisplay(prettyPrinter.writer, "Failed:\n")
		prettyPrinter.printGroupedResources(failedWorkloads)
	}
	if len(excludedWorkloads) > 0 {
		cautils.WarningDisplay(prettyPrinter.writer, "Excluded:\n")
		prettyPrinter.printGroupedResources(excludedWorkloads)
	}
	if len(passedWorkloads) > 0 {
		cautils.SuccessDisplay(prettyPrinter.writer, "Passed:\n")
		prettyPrinter.printGroupedResources(passedWorkloads)
	}

}

func (prettyPrinter *PrettyPrinter) printGroupedResources(workloads map[string][]WorkloadSummary) {
	indent := "  "
	for title, rsc := range workloads {
		prettyPrinter.printGroupedResource(indent, title, rsc)
	}
}

func (prettyPrinter *PrettyPrinter) printGroupedResource(indent string, title string, rsc []WorkloadSummary) {
	preIndent := indent
	if title != "" {
		cautils.SimpleDisplay(prettyPrinter.writer, "%s%s\n", indent, title)
		indent += indent
	}

	resources := []string{}
	for r := range rsc {
		relatedObjectsStr := generateRelatedObjectsStr(rsc[r]) // TODO -
		resources = append(resources, fmt.Sprintf("%s%s - %s %s", indent, rsc[r].resource.GetKind(), rsc[r].resource.GetName(), relatedObjectsStr))
	}

	sort.Strings(resources)
	for i := range resources {
		cautils.SimpleDisplay(prettyPrinter.writer, resources[i]+"\n")
	}

	indent = preIndent
}

func generateRelatedObjectsStr(workload WorkloadSummary) string {
	relatedStr := ""
	if workload.resource.GetObjectType() == workloadinterface.TypeWorkloadObject {
		relatedObjects := objectsenvelopes.NewRegoResponseVectorObject(workload.resource.GetObject()).GetRelatedObjects()
		for i, related := range relatedObjects {
			if ns := related.GetNamespace(); i == 0 && ns != "" {
				relatedStr += fmt.Sprintf("Namespace - %s, ", ns)
			}
			relatedStr += fmt.Sprintf("%s - %s, ", related.GetKind(), related.GetName())
		}
	}
	if relatedStr != "" {
		relatedStr = fmt.Sprintf(" [%s]", relatedStr[:len(relatedStr)-2])
	}
	return relatedStr
}
func generateFooter(summaryDetails *reportsummary.SummaryDetails) []string {
	// Control name | # failed resources | all resources | % success
	row := []string{}
	row = append(row, "Resource Summary") //fmt.Sprintf(""%d", numControlers"))
	row = append(row, fmt.Sprintf("%d", summaryDetails.NumberOfResources().Failed()))
	row = append(row, fmt.Sprintf("%d", summaryDetails.NumberOfResources().Excluded()))
	row = append(row, fmt.Sprintf("%d", summaryDetails.NumberOfResources().All()))
	row = append(row, fmt.Sprintf("%.2f%s", summaryDetails.Score, "%"))
	row = append(row, " ")

	return row
}
func (prettyPrinter *PrettyPrinter) printSummaryTable(summaryDetails *reportsummary.SummaryDetails) {

	summaryTable := tablewriter.NewWriter(prettyPrinter.writer)
	summaryTable.SetAutoWrapText(false)
	summaryTable.SetHeader(getControlTableHeaders())
	summaryTable.SetHeaderLine(true)
	alignments := []int{tablewriter.ALIGN_LEFT, tablewriter.ALIGN_CENTER, tablewriter.ALIGN_CENTER, tablewriter.ALIGN_CENTER, tablewriter.ALIGN_CENTER, tablewriter.ALIGN_CENTER}
	summaryTable.SetColumnAlignment(alignments)
	infoToPrintInfoMap := mapInfoToPrintInfo(summaryDetails.Controls)
	for i := 0; i < len(prettyPrinter.sortedControlNames); i++ {
		summaryTable.Append(generateRow(summaryDetails.Controls.GetControl(reportsummary.EControlCriteriaName, prettyPrinter.sortedControlNames[i]), infoToPrintInfoMap))
	}

	summaryTable.SetFooter(generateFooter(summaryDetails))

	// summaryTable.SetFooter(generateFooter())
	summaryTable.Render()

	prettyPrinter.printInfo(infoToPrintInfoMap)
	// For control scan framework will be nil
	cautils.InfoTextDisplay(prettyPrinter.writer, frameworksScoresToString(summaryDetails.ListFrameworks()))
}

func (prettyPrinter *PrettyPrinter) printInfo(infoToPrintInfoMap map[string]string) {
	for info, stars := range infoToPrintInfoMap {
		cautils.WarningDisplay(prettyPrinter.writer, fmt.Sprintf("%s - %s\n", stars, info))
	}
}

func frameworksScoresToString(frameworks []reportsummary.IFrameworkSummary) string {
	if len(frameworks) == 1 {
		if frameworks[0].GetName() != "" {
			return fmt.Sprintf("FRAMEWORK %s\n", frameworks[0].GetName())
			// cautils.InfoTextDisplay(prettyPrinter.writer, ))
		}
	} else if len(frameworks) > 1 {
		p := "FRAMEWORKS: "
		i := 0
		for ; i < len(frameworks)-1; i++ {
			p += fmt.Sprintf("%s (risk: %.2f), ", frameworks[i].GetName(), frameworks[i].GetScore())
		}
		p += fmt.Sprintf("%s (risk: %.2f)\n", frameworks[i].GetName(), frameworks[i].GetScore())
		return p
	}
	return ""
}

// func getSortedControlsNames(controls []reportsummary.IPolicies) []string {
// 	controlNames := make([]string, 0, len(controls))
// 	for k := range controls {
// 		controlNames = append(controlNames, controls[k].Get())
// 	}
// 	sort.Strings(controlNames)
// 	return controlNames
// }
func getControlLink(controlID string) string {
	return fmt.Sprintf("https://hub.armo.cloud/docs/%s", strings.ToLower(controlID))
}
