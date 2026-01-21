package smoothing

import "container/heap"

// Edge represents a directed edge in the flow network.
type Edge struct {
	To   int // destination node
	Rev  int // index of reverse edge in g[To]
	Cap  int // remaining capacity
	Cost int // cost per unit flow
}

// MinCostFlow implements the successive shortest path algorithm with potentials.
// This avoids negative edges by using Johnson's potential technique.
type MinCostFlow struct {
	n int       // number of nodes
	g [][]*Edge // adjacency list
}

// NewMinCostFlow creates a new MinCostFlow instance with n nodes.
func NewMinCostFlow(n int) *MinCostFlow {
	return &MinCostFlow{
		n: n,
		g: make([][]*Edge, n),
	}
}

// AddEdge adds a directed edge from fr to to with given capacity and cost.
// Also creates the reverse edge with 0 capacity and negative cost.
func (mcf *MinCostFlow) AddEdge(fr, to, cap, cost int) {
	fwd := &Edge{To: to, Rev: len(mcf.g[to]), Cap: cap, Cost: cost}
	rev := &Edge{To: fr, Rev: len(mcf.g[fr]), Cap: 0, Cost: -cost}
	mcf.g[fr] = append(mcf.g[fr], fwd)
	mcf.g[to] = append(mcf.g[to], rev)
}

// dijkstraItem is used in the priority queue for Dijkstra's algorithm.
type dijkstraItem struct {
	dist int
	node int
}

// dijkstraPQ implements heap.Interface for Dijkstra's algorithm.
type dijkstraPQ []dijkstraItem

func (pq dijkstraPQ) Len() int           { return len(pq) }
func (pq dijkstraPQ) Less(i, j int) bool { return pq[i].dist < pq[j].dist }
func (pq dijkstraPQ) Swap(i, j int)      { pq[i], pq[j] = pq[j], pq[i] }
func (pq *dijkstraPQ) Push(x any)        { *pq = append(*pq, x.(dijkstraItem)) }
func (pq *dijkstraPQ) Pop() any {
	old := *pq
	n := len(old)
	x := old[n-1]
	*pq = old[:n-1]
	return x
}

// Flow sends f units of flow from s to t using minimum cost.
// Returns (actual flow sent, total cost).
func (mcf *MinCostFlow) Flow(s, t, f int) (int, int) {
	const INF = 1 << 60
	n := mcf.n
	resCost := 0
	flow := 0

	pot := make([]int, n)   // potential for each node
	dist := make([]int, n)  // shortest distance in current iteration
	prevV := make([]int, n) // previous node
	prevE := make([]int, n) // edge index from previous node

	for flow < f {
		// Initialize distances
		for i := range n {
			dist[i] = INF
		}
		dist[s] = 0

		// Dijkstra with potentials
		pq := &dijkstraPQ{{dist: 0, node: s}}
		heap.Init(pq)

		for pq.Len() > 0 {
			item := heap.Pop(pq).(dijkstraItem)
			v := item.node
			d := item.dist

			if d != dist[v] {
				continue
			}

			for ei, e := range mcf.g[v] {
				if e.Cap <= 0 {
					continue
				}
				// Reduced cost with potential
				nd := d + e.Cost + pot[v] - pot[e.To]
				if nd < dist[e.To] {
					dist[e.To] = nd
					prevV[e.To] = v
					prevE[e.To] = ei
					heap.Push(pq, dijkstraItem{dist: nd, node: e.To})
				}
			}
		}

		// No augmenting path found
		if dist[t] == INF {
			break
		}

		// Update potentials
		for v := range n {
			if dist[v] < INF {
				pot[v] += dist[v]
			}
		}

		// Find minimum capacity along the path
		addFlow := f - flow
		v := t
		for v != s {
			e := mcf.g[prevV[v]][prevE[v]]
			if e.Cap < addFlow {
				addFlow = e.Cap
			}
			v = prevV[v]
		}

		// Augment flow along the path
		v = t
		for v != s {
			e := mcf.g[prevV[v]][prevE[v]]
			e.Cap -= addFlow
			mcf.g[v][e.Rev].Cap += addFlow
			v = prevV[v]
		}

		flow += addFlow
		resCost += addFlow * pot[t]
	}

	return flow, resCost
}
