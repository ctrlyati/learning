# 06 — Multi-Agent Architectures

> **Goal:** Design and build multi-agent systems in LangGraph, configure Supervisor routing patterns, manage context separation, and implement sub-graphs.

---

## 1. Why Multi-Agent Systems?

Putting all tools and instructions into a single "God Agent" often leads to:
1. **Context Window Dilution:** The model gets overwhelmed by long prompts and tool descriptions.
2. **Tool Selection Errors:** The model picks the wrong tool because of similar schemas.
3. **Instruction Drift:** The agent forgets its core objectives during complex tasks.

**Multi-Agent Architectures** solve this by breaking the system down into specialized, independent agents.

```
                  ┌──────────────┐
                  │  Supervisor  │
                  └──────┬───────┘
                         │ (Routes work)
         ┌───────────────┼───────────────┐
         ▼               ▼               ▼
   ┌───────────┐   ┌───────────┐   ┌───────────┐
   │ Researcher│   │   Coder   │   │  Writer   │
   └───────────┘   └───────────┘   └───────────┘
```

---

## 2. Multi-Agent Design Patterns

| Pattern | Control Model | How it Works |
| :--- | :--- | :--- |
| **Supervisor** | Centralized | A orchestrator agent receives user input, decides which sub-agent to call, receives sub-agent output, and repeats until the goal is achieved. |
| **Choreography** | Decentralized | Sub-agents execute tasks and directly hand off control to other agents via custom routing rules in their own nodes. |
| **Sub-graphs** | Hierarchical | An agent is represented as a single node inside a parent graph, encapsulating its own internal state machine. |

---

## 3. Code Example: A Supervisor Multi-Agent System

Let's build a system with two specialized agents (a **Researcher** and a **Coder**) coordinated by a **Supervisor**.

### Step A: Defining the State
We define a shared state that all agents write to.

```python
from typing import Annotated, TypedDict, Literal
from operator import add
from langchain_core.messages import BaseMessage, HumanMessage
from langgraph.graph import StateGraph, START, END

class MultiAgentState(TypedDict):
    messages: Annotated[list[BaseMessage], add]
    next_agent: str  # Dictates who runs next: "Researcher", "Coder", or "FINISH"
```

### Step B: Defining Specialized Agents
Instead of full graphs, we can represent sub-agents as simple LLM calls with distinct prompts.

```python
from langchain_core.prompts import ChatPromptTemplate
from langchain_openai import ChatOpenAI

model = ChatOpenAI(model="gpt-4o-mini")

# Researcher Node
researcher_prompt = ChatPromptTemplate.from_messages([
    ("system", "You are an expert researcher. Search for facts. Be brief."),
    ("placeholder", "{messages}")
])
researcher_runnable = researcher_prompt | model

def researcher_node(state: MultiAgentState):
    response = researcher_runnable.invoke({"messages": state["messages"]})
    # Identify this response as coming from the researcher
    response.name = "Researcher"
    return {"messages": [response], "next_agent": "Supervisor"}

# Coder Node
coder_prompt = ChatPromptTemplate.from_messages([
    ("system", "You write clean, documented Python code. Do not output markdown code blocks."),
    ("placeholder", "{messages}")
])
coder_runnable = coder_prompt | model

def coder_node(state: MultiAgentState):
    response = coder_runnable.invoke({"messages": state["messages"]})
    response.name = "Coder"
    return {"messages": [response], "next_agent": "Supervisor"}
```

### Step C: Defining the Supervisor Node
The supervisor acts as the router. It parses the conversation history and chooses the next agent using structured outputs.

```python
from pydantic import BaseModel, Field

class RouterSchema(BaseModel):
    next: Literal["Researcher", "Coder", "FINISH"] = Field(
        description="Choose the next agent to route to, or FINISH if the user request is fulfilled."
    )

supervisor_prompt = ChatPromptTemplate.from_messages([
    ("system", "You are the manager supervising a Researcher and a Coder. Decide who should work next based on the history."),
    ("placeholder", "{messages}")
])

# Bind structured router schema
supervisor_runnable = supervisor_prompt | model.with_structured_output(RouterSchema)

def supervisor_node(state: MultiAgentState):
    response = supervisor_runnable.invoke({"messages": state["messages"]})
    return {"next_agent": response.next}
```

---

## 4. Assembling the Parent Graph

We link the supervisor's routing decisions to conditional edges.

```python
builder = StateGraph(MultiAgentState)

# Add Nodes
builder.add_node("Supervisor", supervisor_node)
builder.add_node("Researcher", researcher_node)
builder.add_node("Coder", coder_node)

# Define transitions
builder.add_edge(START, "Supervisor")
builder.add_edge("Researcher", "Supervisor")  # Always report back to supervisor
builder.add_edge("Coder", "Supervisor")

# Conditional routing from Supervisor
def route_supervisor(state: MultiAgentState):
    next_agent = state["next_agent"]
    if next_agent == "FINISH":
        return END
    return next_agent

builder.add_conditional_edges(
    "Supervisor",
    route_supervisor,
    {
        "Researcher": "Researcher",
        "Coder": "Coder",
        END: END
    }
)

graph = builder.compile()
```

### Running the Multi-Agent System:
```python
inputs = {"messages": [HumanMessage(content="Find the current population of Tokyo, then write a Python class to model a city with population metrics.")]}
result = graph.invoke(inputs)

for msg in result["messages"]:
    print(f"[{msg.name or 'User'}]: {msg.content}")
```

Expected Behavior:
1. `User` requests information.
2. `Supervisor` sends task to `Researcher`.
3. `Researcher` answers with population data and reports back.
4. `Supervisor` reviews data and routes task to `Coder`.
5. `Coder` generates the Python class based on the researcher's metrics.
6. `Supervisor` terminates (`FINISH`).

---

## 5. Summary Exercises

1. **Exercise 1 (Agent-to-Agent Hand-off):** Rebuild the system using **Choreography** (Decentralized). Remove the `Supervisor` node. Instead, make the `Researcher` node route directly to the `Coder` node based on its own analysis of the result.
2. **Exercise 2 (Sub-graph Isolation):** Research how to add a sub-graph in LangGraph. Create a parent graph that handles general queries, and compile a complete math-loop graph (from Module 04) to act as a single node within this parent graph.

---

* [next → 07_production_and_evals.md](./07_production_and_evals.md)*
* [← back to Module 05](./05_persistence_and_human_in_the_loop.md)*
