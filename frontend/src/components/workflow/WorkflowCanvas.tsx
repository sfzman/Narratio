import React, {useCallback, useEffect} from 'react';
import ReactFlow, { 
  Background, 
  ConnectionLineType,
  Node,
  Edge,
  useNodesState,
  useEdgesState,
  Panel
} from 'reactflow';
import 'reactflow/dist/style.css';
import NodeCard from './NodeCard';
import {WorkflowNode} from '../../types/workflow';
import {Maximize2, ZoomIn, ZoomOut, Play} from 'lucide-react';

const nodeTypes = {
  process: NodeCard,
};

const NODE_POSITIONS: Record<string, {x: number; y: number}> = {
  segmentation: {x: 50, y: 150},
  outline: {x: 50, y: 350},
  character_sheet: {x: 50, y: 550},
  script: {x: 400, y: 350},
  character_image: {x: 400, y: 550},
  tts: {x: 400, y: 150},
  image: {x: 750, y: 450},
  shot_video: {x: 1100, y: 450},
  video: {x: 1450, y: 300},
};

function buildFlowNodes(workflowNodes: WorkflowNode[], selectedNodeId: string | null): Node[] {
  return workflowNodes.map((node) => ({
    id: node.id,
    type: 'process',
    position: NODE_POSITIONS[node.id] || {x: 0, y: 0},
    data: {...node},
    selected: node.id === selectedNodeId,
  }));
}

function buildFlowEdges(workflowNodes: WorkflowNode[]): Edge[] {
  const nodesById = new Map(workflowNodes.map((node) => [node.id, node]));
  const edges: Edge[] = [];

  workflowNodes.forEach((node) => {
    node.dependencies.forEach((dependency) => {
      edges.push({
        id: `e-${dependency}-${node.id}`,
        source: dependency,
        target: node.id,
        type: ConnectionLineType.SmoothStep,
        animated: nodesById.get(dependency)?.status === 'running',
        style: {stroke: '#334155', strokeWidth: 2},
      });
    });
  });

  return edges;
}

interface WorkflowCanvasProps {
  workflowNodes: WorkflowNode[];
  loading?: boolean;
  error?: string | null;
  selectedNodeId: string | null;
  onNodeSelect: (node: WorkflowNode | null) => void;
}

const WorkflowCanvas = ({
  workflowNodes,
  loading = false,
  error = null,
  selectedNodeId,
  onNodeSelect,
}: WorkflowCanvasProps) => {
  const [nodes, setNodes, onNodesChange] = useNodesState([]);
  const [edges, setEdges, onEdgesChange] = useEdgesState([]);

  useEffect(() => {
    setNodes(buildFlowNodes(workflowNodes, selectedNodeId));
    setEdges(buildFlowEdges(workflowNodes));
  }, [workflowNodes, selectedNodeId, setNodes, setEdges]);

  // Sync external selection to ReactFlow nodes
  useEffect(() => {
    setNodes((nds) =>
      nds.map((node) => ({
        ...node,
        selected: node.id === selectedNodeId,
      }))
    );
  }, [selectedNodeId, setNodes]);

  const onNodeClick = useCallback((_: React.MouseEvent, node: Node) => {
    onNodeSelect(node.data as WorkflowNode);
  }, [onNodeSelect]);

  const onPaneClick = useCallback(() => {
    onNodeSelect(null);
  }, [onNodeSelect]);

  return (
    <div className="w-full h-full relative">
      <ReactFlow
        nodes={nodes}
        edges={edges}
        onNodesChange={onNodesChange}
        onEdgesChange={onEdgesChange}
        nodeTypes={nodeTypes}
        onNodeClick={onNodeClick}
        onPaneClick={onPaneClick}
        connectionLineType={ConnectionLineType.SmoothStep}
        fitView
        minZoom={0.2}
        maxZoom={1.5}
      >
        <Background color="#334155" gap={32} size={1} />

        {loading && (
          <Panel position="top-center" className="mt-4 rounded-lg border border-slate-800 bg-slate-900/90 px-4 py-2 text-xs text-slate-300">
            Loading workflow...
          </Panel>
        )}

        {!loading && error && (
          <Panel position="top-center" className="mt-4 rounded-lg border border-rose-900/40 bg-slate-900/90 px-4 py-2 text-xs text-rose-300">
            {error}
          </Panel>
        )}

        {!loading && !error && workflowNodes.length === 0 && (
          <Panel position="top-center" className="mt-4 rounded-lg border border-slate-800 bg-slate-900/90 px-4 py-2 text-xs text-slate-400">
            Select a job to load its workflow.
          </Panel>
        )}
        
        <Panel position="bottom-center" className="bg-slate-900/80 backdrop-blur-md border border-slate-800 p-2 rounded-lg flex items-center gap-4 shadow-2xl mb-4">
          <div className="flex items-center gap-1 border-r border-slate-800 pr-4">
            <button className="p-2 hover:bg-slate-800 rounded-md text-slate-400 transition-colors" title="Zoom In">
              <ZoomIn className="w-4 h-4" />
            </button>
            <button className="p-2 hover:bg-slate-800 rounded-md text-slate-400 transition-colors" title="Zoom Out">
              <ZoomOut className="w-4 h-4" />
            </button>
            <button className="p-2 hover:bg-slate-800 rounded-md text-slate-400 transition-colors" title="Fit View">
              <Maximize2 className="w-4 h-4" />
            </button>
          </div>
          <button className="flex items-center gap-2 bg-brand-cyan hover:bg-cyan-400 text-slate-950 px-4 py-2 rounded-md font-semibold text-sm transition-all shadow-lg shadow-cyan-500/20">
            <Play className="w-4 h-4 fill-current" />
            EXECUTE ALL
          </button>
        </Panel>
      </ReactFlow>
    </div>
  );
};

export default WorkflowCanvas;
