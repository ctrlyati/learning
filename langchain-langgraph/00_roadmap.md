# 00 — LangChain and LangGraph Roadmap

> **Goal:** Take a Python developer from "I can write LLM prompts" to "I design, build, and deploy production-grade cyclic, stateful, multi-agent systems using LangChain and LangGraph."

This tutorial series is designed for software engineers. It skips basic explanations of what an LLM is and focuses on the architectural design patterns, state management models, and verification steps necessary to build autonomous systems that can safely execute code, call tools, self-correct, and scale in production.

---

## Module Table

| #   | Module                                   | Focus                                                                      |
| --- | ---------------------------------------- | -------------------------------------------------------------------------- |
| 01  | LangChain, LCEL, & Runnables             | LangChain Expression Language, Runnables, prompt templates, models, and streaming |
| 02  | Tool Calling & Structured Outputs       | Tool schemas (`@tool`, Pydantic), function calling, parsing, schema enforcement |
| 03  | LangGraph Foundations                    | Statecharts, defining `State`, `Nodes`, `Edges`, compilation, entry/exit points |
| 04  | State Reducers & Conditional Routing     | Overwrite vs. append patterns, parallel execution (fan-out/fan-in), routing|
| 05  | Persistence & Human-in-the-Loop          | Checkpointers (`MemorySaver`, Postgres), Breakpoints, approvals, Time Travel|
| 06  | Multi-Agent Architectures                | Supervisors, networks/choreography, state scoping, sub-graphs              |
| 07  | Production, Guardrails & Evals           | LangSmith, recursion limits, prompt injection, trajectory evals, LLM-as-a-judge |

---

## Timeline & Prerequisites

Study one module per day. Expect to spend 1.5–2 hours per module playing with code examples and building the exercises.

### Prerequisites:
- **Python Fluency:** Especially type hints (`dict[str, Any]`, `Annotated`) and asynchronous programming (`asyncio`, `async`/`await`).
- **Pydantic v2:** You must understand how Pydantic BaseModels validate, parse, and serialize data, as LangChain and LangGraph rely heavily on Pydantic schemas.
- **LLM Basics:** Familiarity with chat completions, system messages, temperature, and tokens.

---

## Core Mental Models

Four critical mental shifts required when moving from basic API calls to LangGraph systems:

1. **LCEL is a Declarative Pipeline, Not Imperative Code.** 
   When writing `chain = prompt | model | parser`, you are not executing code. You are building a directed acyclic graph (DAG) of `Runnable` objects that LangChain compiles under the hood. This enables automatic streaming, parallel execution, batching, and tracing.

2. **LangGraph is a Statechart, Not a Chain.**
   While LangChain represents linear chains of execution, LangGraph represents cyclic graphs. It is modeled on **statecharts** (State + Actions + Transitions). Your code is organized into **Nodes** (which mutate the state) and **Edges** (which route control flow based on the current state).

3. **State is Immutable and Reducer-Driven.**
   In LangGraph, state updates are not performed by directly mutating a global object. Instead, nodes return a dictionary containing keys to update. How these keys are merged into the global state is determined by **reducer functions**. For example, a key can be overwritten, appended to a list, or merged as a set.

4. **The Model is Just a Node.**
   In agentic loops, the LLM is not the orchestrator. It is simply one node in the graph that makes decisions (e.g., "I should call tool X"). The graph orchestrator is responsible for routing, executing tools, managing state history, and enforcing guardrails (like recursion limits).

---

## External Links & Resources

- **[LangChain Documentation](https://python.langchain.com/)** — Reference for components and integrations.
- **[LangGraph Documentation](https://langchain-ai.github.io/langgraph/)** — Explanations of graph architectures, state management, and persistence.
- **[LangSmith](https://www.langchain.com/langsmith)** — Crucial for tracing, debugging agent trajectories, and running automated evaluations.
- **[Pydantic Docs](https://docs.pydantic.dev/latest/)** — Reference for defining tool schemas and structured outputs.

---

* [next → 01_langchain_lcel_and_runnables.md](./01_langchain_lcel_and_runnables.md)*
