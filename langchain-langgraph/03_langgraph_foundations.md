# 03 — LangGraph Foundations

> **Goal:** Understand the statechart execution model of LangGraph, define graph states, implement nodes and edges, and run a compiled state graph.

---

## 1. The Statechart Model of LangGraph

While LangChain is designed for linear DAGs, **LangGraph** is built for **cycles** (loops). This makes it the ideal framework for building autonomous agents that need to plan, call a tool, observe the output, and loop back to adjust their actions.

LangGraph represents your application as a Statechart:
1. **State:** The single source of truth. A schema that holds variables updated throughout the execution.
2. **Nodes:** Plain Python functions or Runnables that receive the current state, perform calculations/API calls, and return updates to the state.
3. **Edges:** Paths connecting nodes.
   *   **Normal Edges:** Always go from Node A to Node B.
   *   **Conditional Edges:** Dynamic routing based on a function that inspects the state.

---

## 2. Defining the Graph State

The state is a shared datastore. You typically define it using Python's `TypedDict` or a Pydantic model. 

```python
from typing import TypedDict, List

class AgentState(TypedDict):
    task: str
    intermediate_steps: List[str]
    final_output: str
```
Every time a Node executes, it returns a dictionary. The orchestrator merges this dictionary with the state. By default, keys are overwritten. If you want to customize how a key is updated, you use **reducers** (covered in Module 04).

---

## 3. Implementing Nodes

A node is a function (sync or async) that takes the state as input and returns a dictionary updating one or more keys of the state.

```python
# Node 1: Planner Node
def planner_node(state: AgentState) -> dict:
    task = state["task"]
    print(f"[Planner] Creating plan for task: {task}")
    step = f"Plan created: Break task '{task}' into 2 sub-tasks."
    return {"intermediate_steps": [step]}

# Node 2: Worker Node
def worker_node(state: AgentState) -> dict:
    steps = state["intermediate_steps"]
    print(f"[Worker] Processing steps. Current log count: {len(steps)}")
    return {"final_output": "Successfully completed task details."}
```

---

## 4. Constructing and Compiling the Graph

Use `StateGraph` to build the graph. Add your nodes, define the entry point, connect the nodes with edges, and compile the graph.

```python
from langgraph.graph import StateGraph, START, END

# 1. Initialize the StateGraph with our state schema
builder = StateGraph(AgentState)

# 2. Add nodes to the graph
builder.add_node("planner", planner_node)
builder.add_node("worker", worker_node)

# 3. Define the connections (edges)
builder.add_edge(START, "planner")     # Entry point
builder.add_edge("planner", "worker")   # Transition edge
builder.add_edge("worker", END)        # Termination edge

# 4. Compile the graph
graph = builder.compile()
```

Once compiled, `graph` is a Runnable! You can call it using `.invoke()`, `.ainvoke()`, or `.stream()`.

---

## 5. Running the Graph

Let's execute our simple sequential graph:

```python
# Run with initial state values
inputs = {"task": "Write an essay about garbage collection in Python."}
result = graph.invoke(inputs)

print("\nFinal Result:")
print("Steps:", result.get("intermediate_steps"))
print("Output:", result.get("final_output"))
```

### Visualizing the Graph
If you have Graphviz installed, you can visualize your graph directly in Python:

```python
try:
    # Get PNG bytes of the graph drawing
    png_bytes = graph.get_graph().draw_mermaid_png()
    with open("graph.png", "wb") as f:
        f.write(png_bytes)
except Exception as e:
    print("Could not generate graph image. Make sure pygraphviz is installed.")
```

---

## 6. Summary Exercises

1. **Exercise 1 (Adding a Node):** Modify the graph above to add a third node called `reviewer_node`. Place it between `worker` and `END`. The reviewer node should read the `final_output` and add a comment like `"Reviewed and Approved!"` to the `final_output`.
2. **Exercise 3 (Async Graph):** Rewrite the nodes to be asynchronous (`async def`) and run the compiled graph asynchronously using `.ainvoke()`.

---

* [next → 04_state_reducers_and_routing.md](./04_state_reducers_and_routing.md)*
* [← back to Module 02](./02_tool_calling_and_structured_outputs.md)*
