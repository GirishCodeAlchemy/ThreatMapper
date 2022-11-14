import { useRef } from 'react';

import {
  getParents,
  IAPIData,
  modelNodeTypeToTopologyChildrenTypes,
  TopologyNodeType,
  uiToServerNodeMap,
  uiToServerNodeParents,
} from '../../topology/utils';
import { UpdateManagerType, useGraphUpdateManager } from '../graphManager/updateManager';
import { LayoutType, useLayoutManager } from '../graphManager/useLayoutManager';
import { GraphItem, IGraph } from '../types';
import { debounce, nodeToFront } from '../utils';
import { topologyEdgesToDelta, topologyNodesToDelta } from './builder';

export const useToplogy = (graph: IGraph | null) => {
  const updateManagerRef = useRef<Partial<UpdateManagerType>>({
    resume: undefined,
  });
  const layoutManagerRef = useRef<LayoutType>();

  // current process node
  const trackedItem = useRef<GraphItem>(null);

  const setTrackedItem = (item: GraphItem) => {
    trackedItem.current = item;
  };

  // create layout manager
  const { layout } = useLayoutManager(graph, {
    tick: debounce(() => {
      if (trackedItem.current) {
        nodeToFront(trackedItem.current);
        graph?.focusItem(trackedItem.current, true);
      }
    }, 500),

    onLayoutStart: () => {
      updateManagerRef.current?.pause?.();
    },

    onLayoutEnd: () => {
      updateManagerRef.current?.resume?.();
    },
  });

  // create graph update manager

  const { updateRootNodes, updateEdges, updateNode, pause, resume } =
    useGraphUpdateManager(graph, layoutManagerRef.current?.layout);

  updateManagerRef.current = {
    pause,
    resume,
  };

  layoutManagerRef.current = {
    layout,
  };

  const update = (data: IAPIData) => {
    if (!graph) {
      return;
    }
    const edges_delta = topologyEdgesToDelta(data.edges);
    const nodes_delta = topologyNodesToDelta(graph, data.nodes);

    if (edges_delta !== null) {
      updateEdges?.({
        add: [],
        remove: edges_delta.remove,
        reset: data.reset,
      });
    }

    if (nodes_delta !== null) {
      let reset = data.reset;
      for (const parent_id of Object.keys(nodes_delta)) {
        if (parent_id === 'root') {
          updateRootNodes?.({
            ...nodes_delta[parent_id],
            reset,
          });
        } else {
          updateNode?.(parent_id, {
            ...nodes_delta[parent_id],
            reset,
          });
        }
        reset = false;
      }
    }
    if (edges_delta !== null) {
      updateEdges?.({ add: edges_delta.add, remove: [] });
    }
    graph?.on('df-track-item', (e) => {
      setTrackedItem(e.item);
    });
  };

  function callExpandApi(item: GraphItem) {
    if (graph === null || item === null) {
      return;
    }
    const node = item.get?.('model');

    const topo_node_type = uiToServerNodeMap(node.node_type);
    if (!topo_node_type) {
      console.error("node can't be expanded", node);
      return;
    }
    const parents = getParents(graph, item).map((id: string) =>
      graph.findById(id).get('model'),
    );

    const nodeTypeIdMapForParent = uiToServerNodeParents(parents);

    const kubernetes =
      nodeTypeIdMapForParent[TopologyNodeType.KUBERNETES_CLUSTER] !== undefined;

    const topo_children_types = modelNodeTypeToTopologyChildrenTypes(node.node_type, {
      kubernetes,
    });
    if (topo_children_types === undefined) {
      console.log('node can not be expanded', node);
      return;
    }
  }

  return {
    update,
  };
};
