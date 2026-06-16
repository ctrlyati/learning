# 05 — Persistence & Human-in-the-Loop

> **Goal:** Implement thread-based session memory using Checkpointers, add manual validation breakpoints (Human-in-the-Loop), and utilize Time Travel to debug and edit graph state history.

---

## 1. Thread Memory & Checkpointers

For agents to run across multiple API requests (or keep chat history), they need **persistence**.

LangGraph manages persistence via **Checkpointers**. When compiled with a checkpointer, the graph automatically saves the state checkpoint after *every* node execution. These checkpoints are grouped under a **thread ID**.

```
User Input (Thread 1) ──> Node A ──> State Checkpoint Saved (Thread 1, Step 1)
                               │
User Input (Thread 2) ──> Node A ──> State Checkpoint Saved (Thread 2, Step 1)
```

### In-Memory Checkpointer (Development)
```python
from langgraph.checkpoint.memory import MemorySaver

# 1. Initialize checkpointer
memory = MemorySaver()

# 2. Compile graph with checkpointer
graph = builder.compile(checkpointer=memory)
```

### Running with a Thread Config
To execute the graph, you must pass a configuration dictionary containing the `thread_id`:

```python
config = {"configurable": {"thread_id": "session-123"}}

# First run: start conversation
inputs = {"messages": [HumanMessage(content="My name is Alex.")]}
graph.invoke(inputs, config)

# Second run: the checkpointer automatically loads the state for session-123
response = graph.invoke({"messages": [HumanMessage(content="What is my name?")]}, config)
print(response["messages"][-1].content)  # Outputs: "Your name is Alex."
```

---

## 2. Human-In-The-Loop (HITL) & Breakpoints

In production systems, you often need a human to review an action before it executes (e.g., executing a bank transfer, running a command).

LangGraph implements this using **Breakpoints**. You tell the compiler to pause the execution `interrupt_before` or `interrupt_after` specific nodes.

```python
# Interrupt the graph BEFORE running the "tools" node
graph_with_interrupt = builder.compile(
    checkpointer=memory,
    interrupt_before=["tools"]
)
```

### The Interrupt / Resume Workflow

```python
config = {"configurable": {"thread_id": "transfer-thread"}}
inputs = {"messages": [HumanMessage(content="Transfer $100 to savings.")]}

# 1. Run the graph. It will execute nodes up to the breakpoint, then pause
state = graph_with_interrupt.invoke(inputs, config)

# Get current state - check if it is interrupted
state_details = graph_with_interrupt.get_state(config)
print("Next Node to execute:", state_details.next)  # Outputs: ("tools",)

# 2. Human reviews the action. If approved, resume the graph by passing None as inputs
print("Resuming execution...")
graph_with_interrupt.invoke(None, config)
```

---

## 3. Time Travel (Auditing and Editing State)

Because LangGraph stores state snapshots at every step, you can retrieve the full history, inspect what happened, rollback the state, or fork the conversation history.

### A. Fetching History
```python
# Get all historic checkpoints for this thread
history = list(graph_with_interrupt.get_state_history(config))
for h in history:
    print(f"Checkpoint ID: {h.config['configurable']['checkpoint_id']} | Next Node: {h.next}")
```

### B. Forking / Modifying State (`update_state`)
You can manually override the state of a running thread. This is useful if an agent gets stuck in a loop, or a human wants to correct a spelling mistake or variable mid-execution.

```python
# Update state manually for a specific checkpoint
graph_with_interrupt.update_state(
    config,
    {"messages": [HumanMessage(content="Actually, transfer $50 instead.")]},
    as_node="agent"  # Mock the update as if it came from the "agent" node
)

# Resume execution using the modified state
graph_with_interrupt.invoke(None, config)
```

---

## 4. Summary Exercises

1. **Exercise 1 (Manual Approvals UI):** Create a mock command-line interface that runs a database tool agent. Using `interrupt_before`, ask the user `[Approve/Reject]` in the console when a database write query tool is called. If approved, execute. If rejected, update the state with a rejection message and resume.
2. **Exercise 2 (Postgres Saver):** Research the `PostgresSaver` configuration. Write out the setup script to initialize a local PostgreSQL docker container and connect LangGraph's checkpointer to it.

---

* [next → 06_multi_agent_architectures.md](./06_multi_agent_architectures.md)*
* [← back to Module 04](./04_state_reducers_and_routing.md)*
