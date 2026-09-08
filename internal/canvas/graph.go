package canvas

import "encoding/json"

type GraphAnalysis struct {
	Components [][]string `json:"components"`
	Orphans    []string   `json:"orphans"`
	Roots      []string   `json:"roots"`
	Leaves     []string   `json:"leaves"`
	Cycles     [][]string `json:"cycles"`
}

func AnalyzeGraph(raw json.RawMessage) (GraphAnalysis, error) {
	var value document
	if err := json.Unmarshal(raw, &value); err != nil {
		return GraphAnalysis{}, invalid("content shape is invalid")
	}
	ids := map[string]bool{}
	for _, item := range value.Nodes {
		ids[item.ID] = true
	}
	outgoing := map[string][]string{}
	incoming := map[string][]string{}
	undirected := map[string][]string{}
	for id := range ids {
		outgoing[id] = []string{}
		incoming[id] = []string{}
		undirected[id] = []string{}
	}
	for _, item := range value.Edges {
		if !ids[item.FromNode] || !ids[item.ToNode] {
			continue
		}
		outgoing[item.FromNode] = append(outgoing[item.FromNode], item.ToNode)
		incoming[item.ToNode] = append(incoming[item.ToNode], item.FromNode)
		undirected[item.FromNode] = append(undirected[item.FromNode], item.ToNode)
		undirected[item.ToNode] = append(undirected[item.ToNode], item.FromNode)
	}
	result := GraphAnalysis{Components: [][]string{}, Orphans: []string{}, Roots: []string{}, Leaves: []string{}, Cycles: [][]string{}}
	visited := map[string]bool{}
	for _, item := range value.Nodes {
		id := item.ID
		if len(incoming[id]) == 0 {
			result.Roots = append(result.Roots, id)
		}
		if len(outgoing[id]) == 0 {
			result.Leaves = append(result.Leaves, id)
		}
		if len(incoming[id]) == 0 && len(outgoing[id]) == 0 {
			result.Orphans = append(result.Orphans, id)
		}
		if visited[id] {
			continue
		}
		component := []string{}
		queue := []string{id}
		visited[id] = true
		for len(queue) > 0 {
			current := queue[0]
			queue = queue[1:]
			component = append(component, current)
			for _, next := range undirected[current] {
				if !visited[next] {
					visited[next] = true
					queue = append(queue, next)
				}
			}
		}
		result.Components = append(result.Components, component)
	}
	state := map[string]int{}
	stack := []string{}
	seenCycles := map[string]bool{}
	var visit func(string)
	visit = func(id string) {
		state[id] = 1
		stack = append(stack, id)
		for _, next := range outgoing[id] {
			if state[next] == 1 {
				start := 0
				for stack[start] != next {
					start++
				}
				cycle := append([]string(nil), stack[start:]...)
				signature := cycleSignature(cycle)
				if !seenCycles[signature] {
					seenCycles[signature] = true
					result.Cycles = append(result.Cycles, cycle)
				}
			} else if state[next] == 0 {
				visit(next)
			}
		}
		stack = stack[:len(stack)-1]
		state[id] = 2
	}
	for id := range ids {
		if state[id] == 0 {
			visit(id)
		}
	}
	return result, nil
}

func cycleSignature(cycle []string) string {
	if len(cycle) == 0 {
		return ""
	}
	minimum := 0
	for index := 1; index < len(cycle); index++ {
		if cycle[index] < cycle[minimum] {
			minimum = index
		}
	}
	result := ""
	for offset := range cycle {
		result += "\x00" + cycle[(minimum+offset)%len(cycle)]
	}
	return result
}
