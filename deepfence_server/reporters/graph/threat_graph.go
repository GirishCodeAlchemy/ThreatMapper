package reporters_graph //nolint:stylecheck

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"time"

	"github.com/deepfence/ThreatMapper/deepfence_utils/directory"
	"github.com/neo4j/neo4j-go-driver/v4/neo4j"
	"github.com/neo4j/neo4j-go-driver/v4/neo4j/dbtype"
)

type ThreatGraphReporter struct {
}

func NewThreatGraphReporter(ctx context.Context) (*ThreatGraphReporter, error) {

	nc := &ThreatGraphReporter{}

	return nc, nil
}

const (
	CloudAws     = "aws"
	CloudAzure   = "azure"
	CloudGcp     = "gcp"
	CloudPrivate = "others"
)

var CloudAll = [...]string{CloudAws, CloudAzure, CloudGcp, CloudPrivate}

func (tc *ThreatGraphReporter) GetThreatGraph(ctx context.Context, filter ThreatFilters) (ThreatGraph, error) {
	aggreg, err := tc.GetRawThreatGraph(ctx, filter)
	if err != nil {
		return ThreatGraph{}, err
	}

	all := ThreatGraph{}
	for _, cp := range CloudAll {
		resources := []ThreatNodeInfo{}
		nodeInfo := aggreg[cp].getNodeInfos()
		depths := aggreg[cp].nodesDepth
		if _, has := depths[1]; !has {
			goto end
		}
		for _, root := range depths[1] {
			visited := map[int64]struct{}{}
			attackPaths := buildAttackPaths(aggreg[cp], root, visited)
			paths := [][]string{}

			for _, attackPath := range attackPaths {
				path := []string{}
				for i := range attackPath {
					index := attackPath[int64(len(attackPath)-1)-int64(i)]
					path = append(path, nodeInfo[index].ID)
				}
				paths = append(paths, append([]string{"The Internet"}, path...))
				index := attackPath[len(attackPath)-1]
				entry := ThreatNodeInfo{
					Label:                nodeInfo[index].Label,
					ID:                   nodeInfo[index].ID,
					Nodes:                nodeInfo[index].Nodes,
					VulnerabilityCount:   nodeInfo[index].VulnerabilityCount,
					SecretsCount:         nodeInfo[index].SecretsCount,
					ComplianceCount:      nodeInfo[index].ComplianceCount,
					CloudComplianceCount: nodeInfo[index].CloudComplianceCount,
					Count:                nodeInfo[index].Count,
					NodeType:             nodeInfo[index].NodeType,
					AttackPath:           paths,
				}
				resources = append(resources, entry)
			}
		}
	end:
		all[cp] = ProviderThreatGraph{
			Resources:            resources,
			ComplianceCount:      0,
			SecretsCount:         0,
			VulnerabilityCount:   0,
			CloudComplianceCount: 0,
		}
	}

	return all, nil
}

func buildAttackPaths(paths AttackPaths, root int64, visited map[int64]struct{}) [][]int64 {
	if _, has := visited[root]; has {
		return [][]int64{}
	}
	visited[root] = struct{}{}
	if _, has := paths.nodesData[root]; !has {
		return [][]int64{}
	}
	if _, has := paths.nodesTree[root]; !has {
		return [][]int64{{root}}
	}
	res := [][]int64{}
	for _, edge := range paths.nodesTree[root] {
		edgePaths := buildAttackPaths(paths, edge, visited)
		for _, edgePath := range edgePaths {
			res = append(res, append([]int64{root}, edgePath...))
		}
	}
	if len(res) == 0 {
		return [][]int64{{root}}
	}
	return res
}

func (tc *ThreatGraphReporter) GetRawThreatGraph(ctx context.Context, filters ThreatFilters) (map[string]AttackPaths, error) {

	driver, err := directory.Neo4jClient(ctx)

	if err != nil {
		return nil, err
	}

	session := driver.NewSession(neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	if err != nil {
		return nil, err
	}
	defer session.Close()

	tx, err := session.BeginTransaction(neo4j.WithTxTimeout(120 * time.Second))
	if err != nil {
		return nil, err
	}
	defer tx.Close()

	// The following statement makes sure all threat graph are exclusively executed.
	// This is required as threat node & threat cloud resource are created on the fly.
	_, err = tx.Run(`
		MERGE (n:ThreatNode{node_id:'root'})
		SET n.lock = true
	`, map[string]interface{}{})
	if err != nil {
		return nil, err
	}

	_, err = tx.Run(`
		MATCH (n:ThreatCloudResource)
		REMOVE n:ThreatCloudResource
	`, map[string]interface{}{})
	if err != nil {
		return nil, err
	}

	_, err = tx.Run(`
		MATCH (n:ThreatNode)
		REMOVE n:ThreatNode
	`, map[string]interface{}{})
	if err != nil {
		return nil, err
	}

	_, err = tx.Run(`
		MATCH (n:CloudResource)
		WHERE n.depth IS NOT NULL
		AND (
			CASE WHEN $type = 'vulnerability' or $type = 'all' THEN n.vulnerabilities_count > 0 ELSE false END
			OR
			CASE WHEN $type = 'secret' or $type = 'all' THEN n.secrets_count > 0 ELSE false END
			OR
			CASE WHEN $type = 'malware' or $type = 'all' THEN n.malwares_count > 0 ELSE false END
			OR
			CASE WHEN $type = 'compliance' or $type = 'all' THEN n.compliances_count > 0 ELSE false END
			OR
			CASE WHEN $type = 'cloud_compliance' or $type = 'all' THEN n.cloud_compliances_count > 0 ELSE false END
		)
		WITH n, n.cloud_provider as provider
		WHERE CASE WHEN size($aws_ids) = 0 OR provider <> 'aws' THEN true ELSE n.account_id IN $aws_ids END
		AND CASE WHEN size($gcp_ids) = 0 OR provider <> 'gcp' THEN true ELSE n.account_id IN $gcp_ids END
		AND CASE WHEN size($azure_ids) = 0 OR provider <> 'azure' THEN true ELSE n.account_id IN $azure_ids END
		SET n:ThreatCloudResource`,
		map[string]interface{}{
			"aws_ids":   filters.AwsFilter.AccountIds,
			"gcp_ids":   filters.GcpFilter.AccountIds,
			"azure_ids": filters.AzureFilter.AccountIds,
			"type":      filters.IssueType,
		},
	)
	if err != nil {
		return nil, err
	}

	_, err = tx.Run(`
		MATCH (n:Node)
		WHERE n.depth IS NOT NULL
		AND (
			CASE WHEN $type = 'vulnerability' or $type = 'all' THEN n.vulnerabilities_count > 0 ELSE false END
			OR
			CASE WHEN $type = 'secret' or $type = 'all' THEN n.secrets_count > 0 ELSE false END
			OR
			CASE WHEN $type = 'malware' or $type = 'all' THEN n.malwares_count > 0 ELSE false END
			OR
			CASE WHEN $type = 'compliance' or $type = 'all' THEN n.compliances_count > 0 ELSE false END
			OR
			CASE WHEN $type = 'cloud_compliance' or $type = 'all' THEN n.cloud_compliances_count > 0 ELSE false END
		)
		SET n:ThreatNode
	`, map[string]interface{}{
		"type": filters.IssueType,
	},
	)
	if err != nil {
		return nil, err
	}

	awsAccountIdsFilterSet := len(filters.AwsFilter.AccountIds) > 0
	gcpAccountIdsFilterSet := len(filters.GcpFilter.AccountIds) > 0
	azureAccountIdsFilterSet := len(filters.AzureFilter.AccountIds) > 0
	cloudAccountIdsFilterSet := awsAccountIdsFilterSet || gcpAccountIdsFilterSet || azureAccountIdsFilterSet

	all := map[string]AttackPaths{}
	for _, cloudProvider := range CloudAll {
		if cloudAccountIdsFilterSet {
			switch cloudProvider {
			case CloudAws:
				if !awsAccountIdsFilterSet {
					continue
				}
			case CloudGcp:
				if !gcpAccountIdsFilterSet {
					continue
				}
			case CloudAzure:
				if !azureAccountIdsFilterSet {
					continue
				}
			case CloudPrivate:
				continue
			}
		}
		var res neo4j.Result
		switch {
		case cloudProvider != CloudPrivate:
			res, err = tx.Run(`
				CALL apoc.nodes.group(['ThreatCloudResource','ThreatNode'], ['node_type', 'depth', 'cloud_provider'],
				[{`+"`*`"+`: 'count', sum_cve: 'sum', sum_secrets: 'sum', sum_compliance: 'sum', sum_cloud_compliance: 'sum',
				node_id:'collect', vulnerabilities_count: 'collect', secrets_count:'collect', compliances_count:'collect', cloud_compliances_count: 'collect'},
				{`+"`*`"+`: 'count'}], {selfRels: false})
				YIELD node, relationships
				WHERE apoc.any.property(node, 'cloud_provider') = '`+cloudProvider+`'
				RETURN node, relationships
				`, map[string]interface{}{})
		case !filters.CloudResourceOnly:
			res, err = tx.Run(`
				CALL apoc.nodes.group(['ThreatNode'], ['node_type', 'depth', 'cloud_provider'],
				[{`+"`*`"+`: 'count', sum_cve: 'sum', sum_secrets: 'sum', sum_compliance: 'sum', sum_cloud_compliance: 'sum',
				node_id:'collect', vulnerabilities_count: 'collect', secrets_count:'collect', compliances_count:'collect', cloud_compliances_count:'collect'},
				{`+"`*`"+`: 'count'}], {selfRels: false})
				YIELD node, relationships
				WHERE NOT apoc.any.property(node, 'cloud_provider') IN ['aws', 'gcp', 'azure']
				AND apoc.any.property(node, 'cloud_provider') <> 'internet'
				RETURN node, relationships
				`, map[string]interface{}{})
		default:
			continue
		}

		if err != nil {
			return nil, err
		}

		records, err := res.Collect()
		if err != nil {
			return nil, err
		}

		nodesTree := map[int64][]int64{}
		nodesData := map[int64]AttackPathData{}
		nodesDepth := map[int64][]int64{}
		for _, record := range records {
			recordNode, _ := record.Get("node")
			recordRelationships, _ := record.Get("relationships")
			node := recordNode.(dbtype.Node)
			nodeDatum := record2struct(node)
			nodesData[node.Id] = nodeDatum

			for _, relNode := range recordRelationships.([]interface{}) {
				rel := relNode.(dbtype.Relationship)
				nodesTree[node.Id] = append(nodesTree[node.Id], rel.EndId)

			}
			nodesDepth[nodeDatum.depth] = append(nodesDepth[nodeDatum.depth], node.Id)
		}

		all[cloudProvider] = AttackPaths{
			nodesTree:  nodesTree,
			nodesData:  nodesData,
			nodesDepth: nodesDepth,
		}
	}

	return all, nil
}

type AttackPaths struct {
	nodesTree  map[int64][]int64
	nodesData  map[int64]AttackPathData
	nodesDepth map[int64][]int64
}

func record2struct(node dbtype.Node) AttackPathData {

	record := node.Props
	nodeType := record["node_type"]
	depth := record["depth"]
	cloudProvider := record["cloud_provider"]
	sumSumCVE := record["sum_sum_cve"]
	sumSumSecrets := record["sum_sum_secrets"]
	sumSumCompliance := record["sum_sum_compliance"]
	sumSumCloudCompliance := record["sum_sum_cloud_compliance"]
	nodeCount := record["count_*"]
	collectNodeID := record["collect_node_id"]
	collectNumCVE := record["collect_vulnerabilities_count"]
	collectNumSecrets := record["collect_secrets_count"]
	collectNumCompliance := record["collect_compliances_count"]
	collectNumCloudCompliance := record["collect_cloud_compliances_count"]

	collectNodeIDs := []string{}
	for _, v := range collectNodeID.([]interface{}) {
		collectNodeIDs = append(collectNodeIDs, v.(string))
	}

	collectNumCVEs := []int64{}
	sumSumCVERes := int64(0)
	if collectNumCVE != nil {
		for _, v := range collectNumCVE.([]interface{}) {
			collectNumCVEs = append(collectNumCVEs, v.(int64))
		}
		sumSumCVERes, _ = sumSumCVE.(int64)
	}

	collectNumSecretsRes := []int64{}
	sumSumSecretsRes := int64(0)
	if collectNumSecrets != nil {
		for _, v := range collectNumSecrets.([]interface{}) {
			collectNumSecretsRes = append(collectNumSecretsRes, v.(int64))
		}
		sumSumSecretsRes = sumSumSecrets.(int64)
	}

	collectNumComplianceRes := []int64{}
	sumSumComplianceRes := int64(0)
	if collectNumCompliance != nil {
		for _, v := range collectNumCompliance.([]interface{}) {
			collectNumComplianceRes = append(collectNumComplianceRes, v.(int64))
		}
		sumSumComplianceRes = sumSumCompliance.(int64)
	}

	collectNumCloudComplianceRes := []int64{}
	sumSumCloudComplianceRes := int64(0)
	if collectNumCloudCompliance != nil {
		for _, v := range collectNumCloudCompliance.([]interface{}) {
			collectNumCloudComplianceRes = append(collectNumCloudComplianceRes, v.(int64))
		}
		sumSumCloudComplianceRes = sumSumCloudCompliance.(int64)
	}

	return AttackPathData{
		identity:                  node.Id,
		NodeType:                  nodeType.(string),
		cloudProvider:             cloudProvider.(string),
		depth:                     depth.(int64),
		sumSumCVE:                 sumSumCVERes,
		sumSumSecrets:             sumSumSecretsRes,
		sumSumCompliance:          sumSumComplianceRes,
		sumSumCloudCompliance:     sumSumCloudComplianceRes,
		nodeCount:                 nodeCount.(int64),
		collectNodeID:             collectNodeIDs,
		collectNumCVE:             collectNumCVEs,
		collectNumSecrets:         collectNumSecretsRes,
		collectNumCompliance:      collectNumComplianceRes,
		collectNumCloudCompliance: collectNumCloudComplianceRes,
	}
}

type AttackPathData struct {
	identity                  int64
	NodeType                  string
	cloudProvider             string
	depth                     int64
	sumSumCVE                 int64
	sumSumSecrets             int64
	sumSumCompliance          int64
	sumSumCloudCompliance     int64
	nodeCount                 int64
	collectNodeID             []string
	collectNumCVE             []int64
	collectNumSecrets         []int64
	collectNumCompliance      []int64
	collectNumCloudCompliance []int64
}

func getThreatNodeID(apd AttackPathData) string {
	h := sha256.New()
	v := []string{}
	v = append(v, apd.collectNodeID...)
	sort.Strings(v)

	for _, s := range v {
		h.Write([]byte(s))
	}

	return hex.EncodeToString(h.Sum(nil))
}

func (ap AttackPaths) getNodeInfos() map[int64]ThreatNodeInfo {
	res := map[int64]ThreatNodeInfo{}
	for _, v := range ap.nodesData {
		var label string
		id := getThreatNodeID(v)
		switch v.NodeType {
		case "host":
			label = "Compute Instance"
		case "container":
			label = "Container"
		case "internet":
			label = "The Internet"
			id = "The Internet"
		default:
			label = "CloudResource"
		}
		Nodes := map[string]NodeInfo{}
		for i, nodeID := range v.collectNodeID {
			vulnCount := int64(0)
			if len(v.collectNumCVE) == len(v.collectNodeID) {
				vulnCount = v.collectNumCVE[i]
			}
			secretsCount := int64(0)
			if len(v.collectNumSecrets) == len(v.collectNodeID) {
				secretsCount = v.collectNumSecrets[i]
			}
			complianceCount := int64(0)
			if len(v.collectNumCompliance) == len(v.collectNodeID) {
				complianceCount = v.collectNumCompliance[i]
			}
			cloudComplianceCount := int64(0)
			if len(v.collectNumCloudCompliance) == len(v.collectNodeID) {
				cloudComplianceCount = v.collectNumCloudCompliance[i]
			}

			Nodes[nodeID] = NodeInfo{
				NodeID:               nodeID,
				Name:                 nodeID,
				VulnerabilityCount:   vulnCount,
				SecretsCount:         secretsCount,
				ComplianceCount:      complianceCount,
				CloudComplianceCount: cloudComplianceCount,
			}
		}
		res[v.identity] = ThreatNodeInfo{
			Label:                label,
			ID:                   id,
			Nodes:                Nodes,
			VulnerabilityCount:   v.sumSumCVE,
			SecretsCount:         v.sumSumSecrets,
			ComplianceCount:      v.sumSumCompliance,
			CloudComplianceCount: v.sumSumCloudCompliance,
			Count:                int64(len(v.collectNodeID)),
			NodeType:             v.NodeType,
			AttackPath:           [][]string{},
		}
	}
	return res
}

type ThreatGraph map[string]ProviderThreatGraph

type ProviderThreatGraph struct {
	Resources            []ThreatNodeInfo `json:"resources" required:"true"`
	ComplianceCount      int64            `json:"compliance_count" required:"true"`
	SecretsCount         int64            `json:"secrets_count" required:"true"`
	VulnerabilityCount   int64            `json:"vulnerability_count" required:"true"`
	CloudComplianceCount int64            `json:"cloud_compliance_count" required:"true"`
}

type ThreatNodeInfo struct {
	Label string              `json:"label" required:"true"`
	ID    string              `json:"id" required:"true"`
	Nodes map[string]NodeInfo `json:"nodes" required:"true"`

	VulnerabilityCount   int64 `json:"vulnerability_count" required:"true"`
	SecretsCount         int64 `json:"secrets_count" required:"true"`
	ComplianceCount      int64 `json:"compliance_count" required:"true"`
	CloudComplianceCount int64 `json:"cloud_compliance_count" required:"true"`
	Count                int64 `json:"count" required:"true"`

	NodeType string `json:"node_type" required:"true"`

	AttackPath [][]string `json:"attack_path" required:"true"`
}

type NodeInfo struct {
	NodeID               string `json:"node_id" required:"true"`
	Name                 string `json:"name" required:"true"`
	VulnerabilityCount   int64  `json:"vulnerability_count" required:"true"`
	SecretsCount         int64  `json:"secrets_count" required:"true"`
	ComplianceCount      int64  `json:"compliance_count" required:"true"`
	CloudComplianceCount int64  `json:"cloud_compliance_count" required:"true"`
}
