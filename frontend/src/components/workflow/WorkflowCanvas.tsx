import React, { useMemo, useCallback, useEffect } from 'react';
import ReactFlow, { 
  Background, 
  Controls, 
  ConnectionLineType,
  Node,
  Edge,
  useNodesState,
  useEdgesState,
  Panel
} from 'reactflow';
import 'reactflow/dist/style.css';
import NodeCard from './NodeCard';
import { WorkflowNode } from '../../types/workflow';
import { DEFAULT_NODES } from '../../constants/workflow';
import { Maximize2, ZoomIn, ZoomOut, Play } from 'lucide-react';

const nodeTypes = {
  process: NodeCard,
};

interface WorkflowCanvasProps {
  selectedNodeId: string | null;
  onNodeSelect: (node: WorkflowNode | null) => void;
}

const WorkflowCanvas = ({ selectedNodeId, onNodeSelect }: WorkflowCanvasProps) => {
  // Initial layout positioning
  const initialNodes: Node[] = useMemo(() => {
    const positions: Record<string, { x: number, y: number }> = {
      segmentation: { x: 50, y: 150 },
      outline: { x: 50, y: 350 },
      character_sheet: { x: 50, y: 550 },
      script: { x: 400, y: 350 },
      character_image: { x: 400, y: 550 },
      tts: { x: 400, y: 150 },
      image: { x: 750, y: 450 },
      shot_video: { x: 1100, y: 450 },
      video: { x: 1450, y: 300 },
    };

    return DEFAULT_NODES.map((node) => ({
      id: node.id,
      type: 'process',
      position: positions[node.id] || { x: 0, y: 0 },
      data: { ...node },
      selected: node.id === selectedNodeId,
    }));
  }, []);

  const initialEdges: Edge[] = useMemo(() => {
    const edges: Edge[] = [];
    DEFAULT_NODES.forEach((node) => {
      node.dependencies.forEach((dep) => {
        edges.push({
          id: `e-${dep}-${node.id}`,
          source: dep,
          target: node.id,
          type: ConnectionLineType.SmoothStep,
          animated: DEFAULT_NODES.find(n => n.id === dep)?.status === 'running',
          style: { stroke: '#334155', strokeWidth: 2 },
        });
      });
    });
    return edges;
  }, []);

  const [nodes, setNodes, onNodesChange] = useNodesState(initialNodes);
  const [edges, setEdges, onEdgesChange] = useEdgesState(initialEdges);

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
