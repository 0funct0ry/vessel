import { useCallback, useEffect, useRef, useState } from "react";
import {
  ReactFlow,
  ReactFlowProvider,
  Background,
  Controls,
  Handle,
  MarkerType,
  Position,
  addEdge,
  useEdgesState,
  useNodesState,
  type Edge,
  type Node,
  type NodeProps,
  type NodeTypes,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { Box, ChevronDown, GitBranch, HardDrive, Link2, Network as NetworkIcon, Plus, RotateCcw, Trash2, X, ZoomIn, ZoomOut } from "lucide-react";
import { Button } from "../components/ui/Button";
import { Modal } from "../components/ui/Modal";
import { EnvVarRow, type EnvRow } from "../components/EnvVarRow";
import { KeyValueRows, mapToRows, rowsToMap, type KVRow } from "../components/KeyValueRows";
import { api } from "../lib/api";
import type { GraphEdge, GraphNode, GraphService, GraphNetworkDef, GraphVolumeDef, StackGraphFromResponse, StackGraphToResponse, StackWarning } from "../types/api";

const inputClass = "mt-1 block w-full rounded border border-line bg-panel px-2 py-1.5 text-[13px]";

type NodeData = { label: string; kind: GraphNode["kind"] };
type FlowNode = Node<NodeData>;
type EdgeData = { kind: GraphEdge["kind"]; mountPath?: string; readOnly?: boolean };
type FlowEdge = Edge<EdgeData>;

const NODE_STYLE: Record<GraphNode["kind"], { border: string; bg: string; text: string; icon: React.ReactNode }> = {
  service: { border: "#7FA8C9", bg: "#EAF1F7", text: "#274A66", icon: <GitBranch size={13} /> },
  network: { border: "#B79FD1", bg: "#F1EBF7", text: "#4A2F66", icon: <NetworkIcon size={13} /> },
  volume: { border: "#8FC1A0", bg: "#EAF6EE", text: "#215A34", icon: <HardDrive size={13} /> },
};

// Edges are drawn by a click-source/click-target gesture (see EdgeDraw
// below), not by dragging a connection handle, so these handles are
// invisible and non-interactive (nodesConnectable={false} on the canvas) —
// but React Flow still needs at least one source and one target handle
// present per node to compute where an edge's line anchors, or it silently
// renders nothing for every edge touching that node.
function GraphNodeBox({ data, selected }: NodeProps<FlowNode>) {
  const style = NODE_STYLE[data.kind];
  return <div style={{ borderColor: style.border, background: style.bg, color: style.text }} className={`min-w-[130px] rounded border px-2.5 py-1.5 text-[12px] shadow-sm ${selected ? "ring-2 ring-hull" : ""}`}>
    <Handle type="source" position={Position.Bottom} className="pointer-events-none opacity-0" />
    <Handle type="target" position={Position.Top} className="pointer-events-none opacity-0" />
    <div className="flex items-center gap-1.5 font-medium">{style.icon}<span className="truncate">{data.label}</span></div>
    <div className="text-[10px] uppercase tracking-wide opacity-70">{data.kind}</div>
  </div>;
}

const NODE_TYPES: NodeTypes = { service: GraphNodeBox, network: GraphNodeBox, volume: GraphNodeBox };

function nodeId(kind: GraphNode["kind"], name: string) { return `${kind}:${name}`; }

function toFlowNode(n: GraphNode, position: { x: number; y: number }): FlowNode {
  return { id: n.id, type: n.kind, position, data: { label: n.name, kind: n.kind }, draggable: true };
}

const EDGE_STYLE: Record<GraphEdge["kind"], Partial<FlowEdge>> = {
  dependency: { style: { stroke: "#8A6D3B", strokeWidth: 1.5 }, markerEnd: { type: MarkerType.ArrowClosed, color: "#8A6D3B" }, animated: false },
  network: { style: { stroke: "#8A67AC", strokeWidth: 1, strokeDasharray: "4 3" } },
  mount: { style: { stroke: "#3E8F5C", strokeWidth: 1.5 } },
};

function toFlowEdge(e: GraphEdge): FlowEdge {
  return { id: e.id, source: e.from, target: e.to, data: { kind: e.kind, mountPath: e.mount_path, readOnly: e.read_only }, ...EDGE_STYLE[e.kind] };
}

type Positions = Record<string, { x: number; y: number }>;

/** Deterministic layered layout: services in a row, each service's networks
 * centered beneath it, each service's named volumes beneath that. No layout
 * dependency (dagre/elkjs) — this is the whole algorithm. */
function layoutTree(nodes: GraphNode[], edges: GraphEdge[]): Positions {
  const services = nodes.filter((n) => n.kind === "service");
  const networks = nodes.filter((n) => n.kind === "network");
  const volumes = nodes.filter((n) => n.kind === "volume");
  const positions: Positions = {};
  const colWidth = 220;

  services.forEach((svc, i) => { positions[svc.id] = { x: i * colWidth, y: 0 }; });

  const placedByRow = (kind: "network" | "volume", y: number, pool: GraphNode[]) => {
    const placed = new Set<string>();
    services.forEach((svc, i) => {
      const linked = edges.filter((e) => e.kind === (kind === "network" ? "network" : "mount") && (e.from === svc.id || e.to === svc.id) && (e.from.startsWith(kind + ":") || e.to.startsWith(kind + ":")));
      let offset = 0;
      for (const e of linked) {
        const otherID = e.from.startsWith(kind + ":") ? e.from : e.to;
        if (placed.has(otherID)) continue;
        positions[otherID] = { x: i * colWidth + offset * 60, y };
        placed.add(otherID);
        offset++;
      }
    });
    let extra = 0;
    for (const n of pool) if (!placed.has(n.id)) { positions[n.id] = { x: services.length * colWidth + extra * 160, y }; extra++; }
  };
  placedByRow("network", 140, networks);
  placedByRow("volume", 280, volumes);

  return positions;
}

/** Row-major grid over every node, sorted by kind then name — no notion of
 * relationships, purely a compact overview shape. */
function layoutGrid(nodes: GraphNode[]): Positions {
  const ordered = [...nodes].sort((a, b) => a.kind.localeCompare(b.kind) || a.name.localeCompare(b.name));
  const cols = Math.max(1, Math.ceil(Math.sqrt(ordered.length)));
  const positions: Positions = {};
  ordered.forEach((n, i) => { positions[n.id] = { x: (i % cols) * 200, y: Math.floor(i / cols) * 130 }; });
  return positions;
}

/** All nodes placed at even angular intervals on one circle. */
function layoutCircle(nodes: GraphNode[]): Positions {
  const positions: Positions = {};
  const radius = Math.max(160, nodes.length * 30);
  nodes.forEach((n, i) => {
    const angle = (2 * Math.PI * i) / Math.max(1, nodes.length);
    positions[n.id] = { x: radius * Math.cos(angle), y: radius * Math.sin(angle) };
  });
  return positions;
}

/** Concentric rings by graph distance from the service set: ring 0 is every
 * service, ring 1 every network/volume directly linked to a service, ring 2
 * anything left over. */
function layoutRadial(nodes: GraphNode[], edges: GraphEdge[]): Positions {
  const services = nodes.filter((n) => n.kind === "service").map((n) => n.id);
  const linked = new Set<string>();
  for (const e of edges) {
    if (services.includes(e.from) && !services.includes(e.to)) linked.add(e.to);
    if (services.includes(e.to) && !services.includes(e.from)) linked.add(e.from);
  }
  const rings: GraphNode[][] = [
    nodes.filter((n) => services.includes(n.id)),
    nodes.filter((n) => linked.has(n.id)),
    nodes.filter((n) => !services.includes(n.id) && !linked.has(n.id)),
  ];
  const positions: Positions = {};
  rings.forEach((ring, ringIndex) => {
    const radius = ringIndex * 200;
    if (radius === 0) { ring.forEach((n, i) => { positions[n.id] = { x: (i - (ring.length - 1) / 2) * 180, y: 0 }; }); return; }
    ring.forEach((n, i) => {
      const angle = (2 * Math.PI * i) / Math.max(1, ring.length);
      positions[n.id] = { x: radius * Math.cos(angle), y: radius * Math.sin(angle) };
    });
  });
  return positions;
}

/** A small fixed-iteration force-directed relaxation: nodes repel each
 * other, connected nodes are pulled toward a resting distance. Run once
 * when selected, not animated per frame — a few dozen iterations of plain
 * vector arithmetic, no layout dependency. */
function layoutForce(nodes: GraphNode[], edges: GraphEdge[]): Positions {
  const positions: Positions = {};
  const n = nodes.length;
  nodes.forEach((node, i) => {
    const angle = (2 * Math.PI * i) / Math.max(1, n);
    positions[node.id] = { x: 200 * Math.cos(angle), y: 200 * Math.sin(angle) };
  });
  const REPULSION = 12000;
  const SPRING_LENGTH = 160;
  const SPRING_STRENGTH = 0.02;
  for (let iter = 0; iter < 60; iter++) {
    const forces: Positions = {};
    for (const node of nodes) forces[node.id] = { x: 0, y: 0 };
    for (let i = 0; i < nodes.length; i++) {
      for (let j = i + 1; j < nodes.length; j++) {
        const a = nodes[i].id, b = nodes[j].id;
        let dx = positions[a].x - positions[b].x;
        let dy = positions[a].y - positions[b].y;
        let distSq = dx * dx + dy * dy;
        if (distSq < 1) { dx = Math.random() - 0.5; dy = Math.random() - 0.5; distSq = 1; }
        const force = REPULSION / distSq;
        const dist = Math.sqrt(distSq);
        forces[a].x += (dx / dist) * force; forces[a].y += (dy / dist) * force;
        forces[b].x -= (dx / dist) * force; forces[b].y -= (dy / dist) * force;
      }
    }
    for (const e of edges) {
      if (!positions[e.from] || !positions[e.to]) continue;
      const dx = positions[e.to].x - positions[e.from].x;
      const dy = positions[e.to].y - positions[e.from].y;
      const dist = Math.max(1, Math.sqrt(dx * dx + dy * dy));
      const stretch = (dist - SPRING_LENGTH) * SPRING_STRENGTH;
      const fx = (dx / dist) * stretch, fy = (dy / dist) * stretch;
      forces[e.from].x += fx; forces[e.from].y += fy;
      forces[e.to].x -= fx; forces[e.to].y -= fy;
    }
    for (const node of nodes) {
      positions[node.id].x += forces[node.id].x;
      positions[node.id].y += forces[node.id].y;
    }
  }
  return positions;
}

export type LayoutKind = "tree" | "grid" | "circle" | "radial" | "force";
const LAYOUT_OPTIONS: { key: LayoutKind; label: string }[] = [
  { key: "tree", label: "Tree" },
  { key: "grid", label: "Grid" },
  { key: "circle", label: "Circle" },
  { key: "radial", label: "Radial" },
  { key: "force", label: "Force" },
];

function computeLayout(kind: LayoutKind, nodes: GraphNode[], edges: GraphEdge[]): Positions {
  switch (kind) {
    case "grid": return layoutGrid(nodes);
    case "circle": return layoutCircle(nodes);
    case "radial": return layoutRadial(nodes, edges);
    case "force": return layoutForce(nodes, edges);
    default: return layoutTree(nodes, edges);
  }
}

function loadStoredLayout(stackKey: string): Record<string, { x: number; y: number }> | null {
  try {
    const raw = localStorage.getItem(`vessel.stackGraphLayout.${stackKey}`);
    return raw ? (JSON.parse(raw) as Record<string, { x: number; y: number }>) : null;
  } catch { return null; }
}

function storeLayout(stackKey: string, positions: Record<string, { x: number; y: number }>) {
  try { localStorage.setItem(`vessel.stackGraphLayout.${stackKey}`, JSON.stringify(positions)); } catch { /* private mode, ignore */ }
}

type AddKind = GraphNode["kind"];

// Dependency and Mount edges are drawn with a click-source, then
// click-target gesture — matching the reference tool exactly — rather than
// dragging between connection handles. sourceId is null until the first
// node is clicked.
type EdgeDrawMode = "dependency" | "mount";
type EdgeDraw = { mode: EdgeDrawMode; sourceId: string | null };

const ADD_KIND_META: Record<AddKind, { label: string; icon: React.ReactNode }> = {
  service: { label: "Service", icon: <Box size={13} className="text-[#274A66]" /> },
  network: { label: "Network", icon: <NetworkIcon size={13} className="text-[#4A2F66]" /> },
  volume: { label: "Volume", icon: <HardDrive size={13} className="text-[#215A34]" /> },
};

/** "Add Service": Name / Image / Ports (comma-separated), matching the
 * reference tool's own Add-Service dialog exactly. A Tailwind modal, never
 * a window.prompt — a service needs more than one field anyway. */
function AddServiceModal({ close, onAdd }: { close: () => void; onAdd: (name: string, image: string, ports: string[]) => void }) {
  const [name, setName] = useState("");
  const [image, setImage] = useState("");
  const [ports, setPorts] = useState("");
  const valid = name.trim().length > 0;
  return <Modal title="Add Service" icon={<Box size={20} className="text-hull" />} close={close}>
    <div className="mt-3 flex flex-col gap-3">
      <label className="block text-[13px]">Name<input autoFocus value={name} onChange={(e) => setName(e.target.value)} placeholder="my-service" className={inputClass} /></label>
      <label className="block text-[13px]">Image<input value={image} onChange={(e) => setImage(e.target.value)} placeholder="nginx:alpine" className={inputClass} /></label>
      <label className="block text-[13px]">Ports (comma-separated)<input value={ports} onChange={(e) => setPorts(e.target.value)} placeholder="8080:80, 443:443" className={inputClass} /></label>
    </div>
    <div className="mt-5 flex justify-end gap-2">
      <Button onClick={close}>Cancel</Button>
      <Button variant="primary" disabled={!valid} onClick={() => onAdd(name.trim(), image.trim(), ports.split(",").map((p) => p.trim()).filter(Boolean))}><Plus size={14} className="mr-1 inline" />Add Service</Button>
    </div>
  </Modal>;
}

/** "Add Network"/"Add Volume": Name only, matching the reference tool. */
function AddSimpleModal({ kind, close, onAdd }: { kind: "network" | "volume"; close: () => void; onAdd: (name: string) => void }) {
  const [name, setName] = useState("");
  const valid = name.trim().length > 0;
  const title = kind === "network" ? "Add Network" : "Add Volume";
  const icon = kind === "network" ? <NetworkIcon size={20} className="text-[#8A67AC]" /> : <HardDrive size={20} className="text-[#3E8F5C]" />;
  const placeholder = kind === "network" ? "my-network" : "my-volume";
  return <Modal title={title} icon={icon} close={close}>
    <div className="mt-3">
      <label className="block text-[13px]">Name<input autoFocus value={name} onChange={(e) => setName(e.target.value)} placeholder={placeholder} className={inputClass} /></label>
    </div>
    <div className="mt-5 flex justify-end gap-2">
      <Button onClick={close}>Cancel</Button>
      <Button variant="primary" disabled={!valid} onClick={() => onAdd(name.trim())}><Plus size={14} className="mr-1 inline" />{title}</Button>
    </div>
  </Modal>;
}

/** Replaces the window.prompt/confirm pair a Mount-mode connection used to
 * open: container path plus a read-only toggle, in one modal. */
function MountDetailModal({ close, onConfirm }: { close: () => void; onConfirm: (path: string, readOnly: boolean) => void }) {
  const [path, setPath] = useState("/data");
  const [readOnly, setReadOnly] = useState(false);
  const valid = path.trim().length > 0;
  return <Modal title="Mount volume" icon={<HardDrive size={20} className="text-[#3E8F5C]" />} close={close}>
    <div className="mt-3 flex flex-col gap-3">
      <label className="block text-[13px]">Container path<input autoFocus value={path} onChange={(e) => setPath(e.target.value)} placeholder="/var/lib/data" className={inputClass} /></label>
      <label className="flex items-center gap-2 text-[13px]"><input type="checkbox" checked={readOnly} onChange={(e) => setReadOnly(e.target.checked)} />Read-only</label>
    </div>
    <div className="mt-5 flex justify-end gap-2">
      <Button onClick={close}>Cancel</Button>
      <Button variant="primary" disabled={!valid} onClick={() => onConfirm(path.trim(), readOnly)}>Mount</Button>
    </div>
  </Modal>;
}

/** The Graph tab's canvas: a two-way node graph of one stack's compose file.
 * Fetches ParseComposeStructure+ToGraph on mount (fresh each time the tab is
 * opened) and only ever re-encodes the compose YAML (FromGraph+Marshal) after
 * an actual edit — never merely for opening the tab — so switching tabs
 * without touching the graph leaves the Editor's YAML untouched. */
export function StackGraph({ stackKey, composeYAML, onComposeYAMLChange, busy }: { stackKey: string; composeYAML: string; onComposeYAMLChange: (yaml: string) => void; busy: boolean }) {
  const [nodes, setNodes, onNodesChange] = useNodesState<FlowNode>([]);
  const [edges, setEdges, onEdgesChange] = useEdgesState<FlowEdge>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [warnings, setWarnings] = useState<StackWarning[]>([]);
  const [selectedID, setSelectedID] = useState<string | null>(null);
  const [edgeDraw, setEdgeDraw] = useState<EdgeDraw | null>(null);
  const [addMenuOpen, setAddMenuOpen] = useState(false);
  const [addModal, setAddModal] = useState<AddKind | null>(null);
  const [pendingMount, setPendingMount] = useState<{ volumeNode: FlowNode; serviceNode: FlowNode } | null>(null);
  const [selectedEdgeID, setSelectedEdgeID] = useState<string | null>(null);
  const [layoutKind, setLayoutKind] = useState<LayoutKind>("tree");
  const [layoutMenuOpen, setLayoutMenuOpen] = useState(false);
  const addMenuRef = useRef<HTMLDivElement>(null);
  const layoutMenuRef = useRef<HTMLDivElement>(null);
  const initialLoad = useRef(true);
  // Node payloads (Service/NetworkDef/VolumeDef) are tracked in a side map
  // rather than React Flow's own node.data, since node.data also carries the
  // label/kind used for rendering.
  const payloads = useRef(new Map<string, GraphService | GraphNetworkDef | GraphVolumeDef>());
  // The most recent compose YAML this component knows the Editor tab to
  // hold — the text /stacks/graph/to was last called with, updated after
  // every successful commit. Sent back as previous_compose_yaml so the
  // server can patch the original document in place instead of doing a
  // full re-marshal that reshuffles every untouched key.
  const latestYAML = useRef(composeYAML);

  useEffect(() => {
    let cancelled = false;
    setLoading(true); setError(null);
    void (async () => {
      try {
        const res = await api.post<StackGraphToResponse>("/stacks/graph/to", { compose_yaml: composeYAML });
        if (cancelled) return;
        const stored = loadStoredLayout(stackKey);
        const computed = computeLayout(layoutKind, res.nodes, res.edges);
        payloads.current = new Map(res.nodes.map((n) => [n.id, (n.service ?? n.network ?? n.volume ?? {}) as GraphService | GraphNetworkDef | GraphVolumeDef]));
        setNodes(res.nodes.map((n) => toFlowNode(n, stored?.[n.id] ?? computed[n.id] ?? { x: 0, y: 0 })));
        setEdges(res.edges.map(toFlowEdge));
        setWarnings(res.warnings);
        setSelectedID(null);
      } catch (err) {
        if (!cancelled) setError(err instanceof Error ? err.message : "could not parse compose file");
      } finally {
        if (!cancelled) { setLoading(false); initialLoad.current = false; }
      }
    })();
    return () => { cancelled = true; };
    // eslint-disable-next-line react-hooks/exhaustive-deps -- fetch once per mount (tab open), not on every keystroke
  }, []);

  const graphNodesOf = useCallback((flow: FlowNode[]): GraphNode[] => flow.map((n) => {
    const payload = payloads.current.get(n.id);
    const base = { id: n.id, kind: n.data.kind, name: n.data.label };
    if (n.data.kind === "service") return { ...base, service: (payload as GraphService) ?? { image: "" } };
    if (n.data.kind === "network") return { ...base, network: (payload as GraphNetworkDef) ?? {} };
    return { ...base, volume: (payload as GraphVolumeDef) ?? {} };
  }), []);

  const commit = useCallback(async (nextNodes: FlowNode[], nextEdges: FlowEdge[]) => {
    const gNodes: GraphNode[] = nextNodes.map((n) => {
      const payload = payloads.current.get(n.id);
      const base = { id: n.id, kind: n.data.kind, name: n.data.label };
      if (n.data.kind === "service") return { ...base, service: (payload as GraphService) ?? { image: "" } };
      if (n.data.kind === "network") return { ...base, network: (payload as GraphNetworkDef) ?? {} };
      return { ...base, volume: (payload as GraphVolumeDef) ?? {} };
    });
    const gEdges: GraphEdge[] = nextEdges.map((e) => ({ id: e.id, kind: (e.data?.kind ?? "network"), from: e.source, to: e.target, mount_path: e.data?.mountPath, read_only: e.data?.readOnly }));
    try {
      const res = await api.post<StackGraphFromResponse>("/stacks/graph/from", { nodes: gNodes, edges: gEdges, previous_compose_yaml: latestYAML.current });
      latestYAML.current = res.compose_yaml;
      onComposeYAMLChange(res.compose_yaml);
    } catch (err) {
      setError(err instanceof Error ? err.message : "could not apply graph edit");
    }
  }, [onComposeYAMLChange]);

  function persistLayout(flow: FlowNode[]) {
    const positions: Record<string, { x: number; y: number }> = {};
    for (const n of flow) positions[n.id] = n.position;
    storeLayout(stackKey, positions);
  }

  const onNodeDragStop = useCallback((_: unknown, node: FlowNode) => { persistLayout([...nodes.filter((n) => n.id !== node.id), node]); }, [nodes, stackKey]); // eslint-disable-line react-hooks/exhaustive-deps

  function cancelEdgeDraw() { setEdgeDraw(null); }

  function toggleEdgeDrawMode(mode: EdgeDrawMode) {
    setEdgeDraw((current) => (current?.mode === mode ? null : { mode, sourceId: null }));
  }

  // Click-source, then click-target: the first click on a node while a mode
  // is active records it as the source; the second click (on a different
  // node) attempts to complete the edge. Falls through to ordinary node
  // selection (the property panel) when no edge-draw mode is active.
  function handleNodeClick(node: FlowNode) {
    if (!edgeDraw) { setSelectedID(node.id); return; }

    if (!edgeDraw.sourceId) {
      setEdgeDraw({ ...edgeDraw, sourceId: node.id });
      return;
    }
    if (node.id === edgeDraw.sourceId) return; // clicking the source again is a no-op, still awaiting a target

    const source = nodes.find((n) => n.id === edgeDraw.sourceId);
    if (!source) { setEdgeDraw({ ...edgeDraw, sourceId: null }); return; }

    if (edgeDraw.mode === "dependency") {
      if (source.data.kind !== "service" || node.data.kind !== "service") {
        setError("Dependency edges connect two services.");
        setEdgeDraw({ mode: "dependency", sourceId: null });
        return;
      }
      const edge: FlowEdge = { id: `dependency:${source.id}:${node.id}`, source: source.id, target: node.id, data: { kind: "dependency" }, ...EDGE_STYLE.dependency };
      setEdges((current) => { const next = addEdge(edge, current) as FlowEdge[]; void commit(nodes, next); return next; });
      setEdgeDraw(null);
      return;
    }

    // Mount mode: either click order is accepted — the service/resource
    // pair is sorted out from node kind, not from click order. Widened to
    // accept a network target too (not just a volume), matching the
    // reference tool's own "Mount" gesture, which is really a generic
    // service<->resource attach tool.
    const serviceNode = source.data.kind === "service" ? source : node.data.kind === "service" ? node : null;
    const resourceNode = source.data.kind !== "service" ? source : node.data.kind !== "service" ? node : null;
    if (!serviceNode || !resourceNode || serviceNode.id === resourceNode.id) {
      setError("Mount edges connect a service and a network or volume.");
      setEdgeDraw({ mode: "mount", sourceId: null });
      return;
    }
    if (resourceNode.data.kind === "network") {
      const edge: FlowEdge = { id: `network:${serviceNode.id}:${resourceNode.id}`, source: serviceNode.id, target: resourceNode.id, data: { kind: "network" }, ...EDGE_STYLE.network };
      setEdges((current) => { const next = addEdge(edge, current) as FlowEdge[]; void commit(nodes, next); return next; });
    } else {
      setPendingMount({ volumeNode: resourceNode, serviceNode });
    }
    setEdgeDraw(null);
  }

  useEffect(() => {
    function onDocumentMouseDown(e: MouseEvent) {
      if (addMenuRef.current && !addMenuRef.current.contains(e.target as globalThis.Node)) setAddMenuOpen(false);
      if (layoutMenuRef.current && !layoutMenuRef.current.contains(e.target as globalThis.Node)) setLayoutMenuOpen(false);
    }
    document.addEventListener("mousedown", onDocumentMouseDown);
    return () => document.removeEventListener("mousedown", onDocumentMouseDown);
  }, []);

  function insertNode(kind: AddKind, name: string, payload: GraphService | GraphNetworkDef | GraphVolumeDef) {
    const id = nodeId(kind, name);
    if (nodes.some((n) => n.id === id)) { setError(`A ${kind} named ${name} already exists.`); return; }
    payloads.current.set(id, payload);
    const position = { x: 40 + nodes.length * 20, y: 40 + nodes.length * 10 };
    const next = [...nodes, toFlowNode({ id, kind, name }, position)];
    setNodes(next);
    void commit(next, edges);
    setAddModal(null);
  }

  function addServiceNode(name: string, image: string, ports: string[]) {
    insertNode("service", name, { image, ports } as GraphService);
  }

  function addSimpleNode(kind: "network" | "volume", name: string) {
    insertNode(kind, name, {} as GraphNetworkDef | GraphVolumeDef);
  }

  function confirmMount(path: string, readOnly: boolean) {
    if (!pendingMount) return;
    const { volumeNode, serviceNode } = pendingMount;
    const edge: FlowEdge = { id: `mount:${volumeNode.id}:${serviceNode.id}:${path}`, source: volumeNode.id, target: serviceNode.id, data: { kind: "mount", mountPath: path, readOnly }, ...EDGE_STYLE.mount };
    setEdges((current) => { const next = addEdge(edge, current) as FlowEdge[]; void commit(nodes, next); return next; });
    setPendingMount(null);
  }

  function deleteSelected() {
    if (!selectedID) return;
    const next = nodes.filter((n) => n.id !== selectedID);
    const nextEdges = edges.filter((e) => e.source !== selectedID && e.target !== selectedID);
    setNodes(next); setEdges(nextEdges);
    payloads.current.delete(selectedID);
    setSelectedID(null);
    void commit(next, nextEdges);
  }

  // Clicking an edge selects it (mutually exclusive with node selection) and
  // opens a small panel — matching the reference tool — with just a delete
  // action. Ignored while a Dependency/Mount click-gesture is in progress,
  // since edges aren't valid click targets for that gesture.
  function handleEdgeClick(edge: FlowEdge) {
    if (edgeDraw) return;
    setSelectedEdgeID(edge.id);
    setSelectedID(null);
  }

  function deleteEdge(id: string) {
    const next = edges.filter((e) => e.id !== id);
    setEdges(next);
    setSelectedEdgeID(null);
    void commit(nodes, next);
  }

  function runLayout(kind: LayoutKind) {
    localStorage.removeItem(`vessel.stackGraphLayout.${stackKey}`);
    const gNodes = graphNodesOf(nodes);
    const gEdges: GraphEdge[] = edges.map((e) => ({ id: e.id, kind: e.data?.kind ?? "network", from: e.source, to: e.target, mount_path: e.data?.mountPath, read_only: e.data?.readOnly }));
    const computed = computeLayout(kind, gNodes, gEdges);
    setNodes(nodes.map((n) => ({ ...n, position: computed[n.id] ?? n.position })));
  }

  function applyLayoutKind(kind: LayoutKind) {
    setLayoutKind(kind);
    setLayoutMenuOpen(false);
    runLayout(kind);
  }

  function resetLayout() {
    runLayout(layoutKind);
  }

  const selected = nodes.find((n) => n.id === selectedID) ?? null;

  function updateSelectedPayload(patch: Partial<GraphService> | Partial<GraphNetworkDef> | Partial<GraphVolumeDef>) {
    if (!selected) return;
    const current = payloads.current.get(selected.id) ?? {};
    const next = { ...current, ...patch };
    payloads.current.set(selected.id, next);
    void commit(nodes, edges);
  }

  const selectedEdge = edges.find((e) => e.id === selectedEdgeID) ?? null;

  return <div className="flex min-h-0 flex-1">
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="flex items-center gap-1 border-b border-linesoft px-3 py-1.5">
        <div className="relative" ref={addMenuRef}>
          <button type="button" onClick={() => setAddMenuOpen((v) => !v)} aria-haspopup="menu" aria-expanded={addMenuOpen} className="flex items-center gap-1 rounded border border-line bg-panel px-2 py-1 text-[12px] text-text">
            <Plus size={13} /> Add <ChevronDown size={12} />
          </button>
          {addMenuOpen && <div role="menu" className="absolute left-0 top-full z-10 mt-1 w-36 rounded border border-line bg-panel py-1 shadow-lg">
            {(["service", "network", "volume"] as AddKind[]).map((kind) => (
              <button key={kind} type="button" role="menuitem" onClick={() => { setAddModal(kind); setAddMenuOpen(false); }} className="flex w-full items-center gap-2 px-3 py-1.5 text-left text-[12.5px] hover:bg-paper">
                {ADD_KIND_META[kind].icon}{ADD_KIND_META[kind].label}
              </button>
            ))}
          </div>}
        </div>
        <button type="button" title="Click a source service, then a target service" onClick={() => toggleEdgeDrawMode("dependency")} className={`flex items-center gap-1 rounded border px-2 py-1 text-[12px] ${edgeDraw?.mode === "dependency" ? "border-hull bg-hull/10 text-hull" : "border-line text-muted"}`}>
          {edgeDraw?.mode === "dependency" ? <><X size={13} /> Cancel</> : <><GitBranch size={13} /> Dependency</>}
        </button>
        <button type="button" title="Click a service, then a network or volume (or the other order)" onClick={() => toggleEdgeDrawMode("mount")} className={`flex items-center gap-1 rounded border px-2 py-1 text-[12px] ${edgeDraw?.mode === "mount" ? "border-hull bg-hull/10 text-hull" : "border-line text-muted"}`}>
          {edgeDraw?.mode === "mount" ? <><X size={13} /> Cancel</> : <><Link2 size={13} /> Mount</>}
        </button>
        {edgeDraw && <span className="rounded-sm border border-hull/30 bg-hull/5 px-2 py-1 text-[11.5px] text-hull">
          {edgeDraw.sourceId ? "Click target service" : edgeDraw.mode === "dependency" ? "Click source service" : "Click a service, network, or volume"}
        </span>}
        <span className="flex-1" />
        <button type="button" title="Delete selected node" disabled={!selectedID} onClick={deleteSelected} className="rounded p-1.5 text-muted hover:bg-paper hover:text-fail disabled:opacity-40"><Trash2 size={14} /></button>
        <div className="relative" ref={layoutMenuRef}>
          <button type="button" title="Layout algorithm" onClick={() => setLayoutMenuOpen((v) => !v)} aria-haspopup="menu" aria-expanded={layoutMenuOpen} className="flex items-center gap-1 rounded border border-line bg-panel px-2 py-1 text-[12px] text-text">
            {LAYOUT_OPTIONS.find((o) => o.key === layoutKind)?.label} <ChevronDown size={12} />
          </button>
          {layoutMenuOpen && <div role="menu" className="absolute right-0 top-full z-10 mt-1 w-28 rounded border border-line bg-panel py-1 shadow-lg">
            {LAYOUT_OPTIONS.map((option) => (
              <button key={option.key} type="button" role="menuitem" onClick={() => applyLayoutKind(option.key)} className={`block w-full px-3 py-1.5 text-left text-[12.5px] hover:bg-paper ${option.key === layoutKind ? "font-medium text-hull" : ""}`}>
                {option.label}
              </button>
            ))}
          </div>}
        </div>
        <button type="button" title="Reset layout" onClick={resetLayout} className="rounded p-1.5 text-muted hover:bg-paper hover:text-text"><RotateCcw size={14} /></button>
      </div>

      <p className="m-0 border-b border-linesoft bg-paper px-3 py-1 text-[11px] text-muted">Re-encoding the compose file after a graph edit does not preserve comments or original formatting.</p>

      {error && <p className="m-0 border-b border-fail/30 bg-paper px-3 py-1 text-[12px] text-fail">{error}</p>}
      {warnings.length > 0 && <p className="m-0 border-b border-pause/30 bg-paper px-3 py-1 text-[11px] text-muted">{warnings.length} warning(s) — see the Editor tab.</p>}

      <div className="min-h-0 flex-1">
        {loading ? <div className="grid h-full place-items-center text-[13px] text-muted">Parsing compose file…</div> : <ReactFlowProvider>
          <ReactFlow
            nodes={nodes.map((n) => ({ ...n, selected: n.id === edgeDraw?.sourceId || n.id === selectedID }))}
            edges={edges.map((e) => ({ ...e, selected: e.id === selectedEdgeID }))}
            onNodesChange={onNodesChange}
            onEdgesChange={onEdgesChange}
            onNodeDragStop={onNodeDragStop}
            onNodeClick={(_, node) => handleNodeClick(node)}
            onEdgeClick={(_, edge) => handleEdgeClick(edge)}
            onPaneClick={() => { if (edgeDraw) cancelEdgeDraw(); setSelectedID(null); setSelectedEdgeID(null); }}
            nodeTypes={NODE_TYPES}
            nodesConnectable={false}
            nodesDraggable={!busy}
            proOptions={{ hideAttribution: true }}
            fitView
          >
            <Background gap={16} />
            <Controls showInteractive={false} position="bottom-left">
              <button type="button" title="Zoom in" className="react-flow__controls-button"><ZoomIn size={12} /></button>
              <button type="button" title="Zoom out" className="react-flow__controls-button"><ZoomOut size={12} /></button>
            </Controls>
          </ReactFlow>
        </ReactFlowProvider>}
      </div>
      <div className="flex items-center gap-3 border-t border-linesoft px-3 py-1.5 text-[11px] text-muted">
        <span className="flex items-center gap-1"><span className="inline-block h-2.5 w-2.5 rounded-sm" style={{ background: NODE_STYLE.service.bg, border: `1px solid ${NODE_STYLE.service.border}` }} />Service</span>
        <span className="flex items-center gap-1"><span className="inline-block h-2.5 w-2.5 rounded-sm" style={{ background: NODE_STYLE.network.bg, border: `1px solid ${NODE_STYLE.network.border}` }} />Network</span>
        <span className="flex items-center gap-1"><span className="inline-block h-2.5 w-2.5 rounded-sm" style={{ background: NODE_STYLE.volume.bg, border: `1px solid ${NODE_STYLE.volume.border}` }} />Volume</span>
        <span className="ml-auto" />
        <span>— dependency &nbsp; ┄ network &nbsp; — mount</span>
      </div>
    </div>

    {selected && <PropertyPanel
      key={selected.id}
      node={selected}
      payload={payloads.current.get(selected.id)}
      onChange={updateSelectedPayload}
      disabled={busy}
    />}

    {selectedEdge && <EdgePanel
      edge={selectedEdge}
      fromLabel={nodes.find((n) => n.id === selectedEdge.source)?.data.label ?? selectedEdge.source}
      toLabel={nodes.find((n) => n.id === selectedEdge.target)?.data.label ?? selectedEdge.target}
      onDelete={() => deleteEdge(selectedEdge.id)}
    />}

    {addModal === "service" && <AddServiceModal close={() => setAddModal(null)} onAdd={addServiceNode} />}
    {(addModal === "network" || addModal === "volume") && <AddSimpleModal kind={addModal} close={() => setAddModal(null)} onAdd={(name) => addSimpleNode(addModal, name)} />}
    {pendingMount && <MountDetailModal close={() => setPendingMount(null)} onConfirm={confirmMount} />}
  </div>;
}

// Short-form `source:target[:ro]` volume entries <-> Host/Container rows.
// The read-only flag isn't exposed as a per-row toggle here (matching the
// reference tool's own property panel, which doesn't expose it either) but
// is preserved silently across edits rather than dropped.
function volumesToRows(volumes: string[] | undefined): { rows: KVRow[]; ro: boolean[] } {
  const rows: KVRow[] = []; const ro: boolean[] = [];
  for (const entry of volumes ?? []) {
    const parts = entry.split(":");
    rows.push({ a: parts[0] ?? "", b: parts[1] ?? "" });
    ro.push(parts[2] === "ro");
  }
  return { rows, ro };
}

function rowsToVolumes(rows: KVRow[], ro: boolean[]): string[] {
  return rows.filter((r) => r.a || r.b).map((r, i) => (ro[i] ? `${r.a}:${r.b}:ro` : `${r.a}:${r.b}`));
}

function PropertyPanel({ node, payload, onChange, disabled }: {
  node: FlowNode;
  payload: GraphService | GraphNetworkDef | GraphVolumeDef | undefined;
  onChange: (patch: Partial<GraphService> | Partial<GraphNetworkDef> | Partial<GraphVolumeDef>) => void;
  disabled: boolean;
}) {
  const kind = node.data.kind;
  const input = "mt-1 block w-full rounded border border-line bg-panel px-2 py-1 text-[12.5px]";

  if (kind === "service") {
    const svc = (payload ?? {}) as GraphService;
    const envRows: EnvRow[] = Object.entries(svc.env ?? {}).map(([key, value]) => ({ key, value }));
    const { rows: volumeRows, ro: volumeRO } = volumesToRows(svc.volumes);
    return <div className="w-[280px] shrink-0 overflow-auto border-l border-line p-3 text-[12.5px]">
      <h3 className="m-0 mb-2 text-[13px] font-medium">{node.data.label} <span className="text-muted">(service)</span></h3>
      <label>Image<input disabled={disabled} className={input} value={svc.image ?? ""} onChange={(e) => onChange({ image: e.target.value })} /></label>
      <label className="mt-2 block">Command<input disabled={disabled} className={input} value={svc.command ?? ""} onChange={(e) => onChange({ command: e.target.value })} /></label>
      <label className="mt-2 block">Entrypoint<input disabled={disabled} className={input} value={svc.entrypoint ?? ""} onChange={(e) => onChange({ entrypoint: e.target.value })} /></label>
      <label className="mt-2 block">Restart policy<input disabled={disabled} className={input} value={svc.restart ?? ""} onChange={(e) => onChange({ restart: e.target.value })} /></label>

      <div className="mt-3">
        <div className="mb-1 text-muted">Port mappings</div>
        <KeyValueRows rows={(svc.ports ?? []).map((p) => { const at = p.lastIndexOf(":"); return at < 0 ? { a: p, b: "" } : { a: p.slice(0, at), b: p.slice(at + 1) }; })} labelA="Host" labelB="Container" disabled={disabled} addLabel="Add port"
          onChange={(rows) => onChange({ ports: rows.filter((r) => r.a || r.b).map((r) => (r.b ? `${r.a}:${r.b}` : r.a)) })} />
      </div>

      <div className="mt-3">
        <div className="mb-1 text-muted">Volumes</div>
        <KeyValueRows rows={volumeRows} labelA="Host" labelB="Container" disabled={disabled} addLabel="Add volume"
          onChange={(rows) => onChange({ volumes: rowsToVolumes(rows, volumeRO) })} />
      </div>

      <div className="mt-3">
        <div className="mb-1 text-muted">Environment</div>
        {envRows.length === 0 && <p className="m-0 text-muted">No environment variables.</p>}
        {envRows.map((row, i) => <EnvVarRow key={i} row={row} disabled={disabled} onChange={(next) => { const rows = [...envRows]; rows[i] = next; onChange({ env: Object.fromEntries(rows.map((r) => [r.key, r.value])) }); }} onRemove={() => { const rows = envRows.filter((_, idx) => idx !== i); onChange({ env: Object.fromEntries(rows.map((r) => [r.key, r.value])) }); }} />)}
        <button type="button" disabled={disabled} className="mt-1 text-hull hover:underline" onClick={() => onChange({ env: { ...(svc.env ?? {}), "": "" } })}><Plus size={12} className="mr-1 inline" />Add variable</button>
      </div>

      <div className="mt-3">
        <div className="mb-1 text-muted">Labels</div>
        <KeyValueRows rows={mapToRows(svc.labels)} labelA="Key" labelB="Value" disabled={disabled} addLabel="Add label" onChange={(rows) => onChange({ labels: rowsToMap(rows) })} />
      </div>

      <p className="mt-3 text-muted">Network membership is edited from the canvas — use the Mount tool to attach this service to a network, or click the network edge to remove it.</p>
    </div>;
  }

  if (kind === "network") {
    const net = (payload ?? {}) as GraphNetworkDef;
    return <div className="w-[280px] shrink-0 overflow-auto border-l border-line p-3 text-[12.5px]">
      <h3 className="m-0 mb-2 text-[13px] font-medium">{node.data.label} <span className="text-muted">(network)</span></h3>
      <label>Driver<input disabled={disabled} className={input} value={net.driver ?? ""} onChange={(e) => onChange({ driver: e.target.value })} /></label>
      <label className="mt-2 block">Subnet<input disabled={disabled} className={input} value={net.ipam_subnet ?? ""} onChange={(e) => onChange({ ipam_subnet: e.target.value })} /></label>
      <label className="mt-2 block">Gateway<input disabled={disabled} className={input} value={net.ipam_gateway ?? ""} onChange={(e) => onChange({ ipam_gateway: e.target.value })} /></label>
      <label className="mt-2 flex items-center gap-2"><input type="checkbox" disabled={disabled} checked={!!net.internal} onChange={(e) => onChange({ internal: e.target.checked })} />Internal</label>
      <label className="mt-2 flex items-center gap-2"><input type="checkbox" disabled={disabled} checked={!!net.attachable} onChange={(e) => onChange({ attachable: e.target.checked })} />Attachable</label>
      <div className="mt-3">
        <div className="mb-1 text-muted">Driver options</div>
        <KeyValueRows rows={mapToRows(net.driver_opts)} labelA="Key" labelB="Value" disabled={disabled} addLabel="Add option" onChange={(rows) => onChange({ driver_opts: rowsToMap(rows) })} />
      </div>
      <div className="mt-3">
        <div className="mb-1 text-muted">Labels</div>
        <KeyValueRows rows={mapToRows(net.labels)} labelA="Key" labelB="Value" disabled={disabled} addLabel="Add label" onChange={(rows) => onChange({ labels: rowsToMap(rows) })} />
      </div>
    </div>;
  }

  const vol = (payload ?? {}) as GraphVolumeDef;
  return <div className="w-[280px] shrink-0 overflow-auto border-l border-line p-3 text-[12.5px]">
    <h3 className="m-0 mb-2 text-[13px] font-medium">{node.data.label} <span className="text-muted">(volume)</span></h3>
    <label>Driver<input disabled={disabled} className={input} value={vol.driver ?? ""} onChange={(e) => onChange({ driver: e.target.value })} /></label>
    <label className="mt-2 flex items-center gap-2"><input type="checkbox" disabled={disabled} checked={!!vol.external} onChange={(e) => onChange({ external: e.target.checked })} />External</label>
    <div className="mt-3">
      <div className="mb-1 text-muted">Driver options</div>
      <KeyValueRows rows={mapToRows(vol.driver_opts)} labelA="Key" labelB="Value" disabled={disabled} addLabel="Add option" onChange={(rows) => onChange({ driver_opts: rowsToMap(rows) })} />
    </div>
    <div className="mt-3">
      <div className="mb-1 text-muted">Labels</div>
      <KeyValueRows rows={mapToRows(vol.labels)} labelA="Key" labelB="Value" disabled={disabled} addLabel="Add label" onChange={(rows) => onChange({ labels: rowsToMap(rows) })} />
    </div>
  </div>;
}

const EDGE_KIND_META: Record<GraphEdge["kind"], { title: string; description: string }> = {
  dependency: { title: "Dependency", description: "The source service is started before the target service that depends on it." },
  network: { title: "Network Connection", description: "Service connected to this network." },
  mount: { title: "Volume Mount", description: "Volume mounted to this service." },
};

/** Clicking an edge opens this compact panel — matching the reference tool
 * — with just enough context to identify the edge and a delete action. */
function EdgePanel({ edge, fromLabel, toLabel, onDelete }: { edge: FlowEdge; fromLabel: string; toLabel: string; onDelete: () => void }) {
  const meta = EDGE_KIND_META[edge.data?.kind ?? "network"];
  return <div className="w-[280px] shrink-0 overflow-auto border-l border-line p-3 text-[12.5px]">
    <div className="flex items-center justify-between">
      <h3 className="m-0 text-[13px] font-medium">{meta.title}</h3>
      <button type="button" title="Delete" aria-label={`Delete ${meta.title.toLowerCase()}`} onClick={onDelete} className="rounded p-1 text-muted hover:bg-paper hover:text-fail"><Trash2 size={14} /></button>
    </div>
    <p className="mt-1 mb-0 font-mono text-[12px] text-muted">{fromLabel} → {toLabel}</p>
    <p className="mt-2 mb-0 text-muted">{meta.description}</p>
  </div>;
}
