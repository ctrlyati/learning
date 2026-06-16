# 07 — Production, Guardrails & Evals

> **Goal:** Deploy LangGraph projects to production, implement execution guardrails, monitor agents with LangSmith, and set up trajectory-based evaluation pipelines.

---

## 1. Observability: Tracing with LangSmith

Debugging agent loops without tracing is incredibly difficult because agents run recursively, call tools, and alter state behind the scenes. 

**LangSmith** provides visual execution graphs showing exactly what prompt was sent to the model, what tool arguments were generated, and what state was updated at each step.

### Activating Tracing
No code modification is needed. Simply set these environment variables in your environment:

```bash
export LANGCHAIN_TRACING_V2="true"
export LANGCHAIN_PROJECT="my-agentic-app"
export LANGCHAIN_API_KEY="your-langsmith-api-key"
```

Once set, any execution of a LangChain or LangGraph runnable will automatically stream trace logs to your LangSmith dashboard.

---

## 2. Guardrails: Protecting Production Systems

Autonomous agents can execute dangerous instructions, get stuck in infinite loops, or run up massive API bills. Implement these safeguards:

### A. Recursion Limits
Always enforce a step limit when invoking graphs. If the graph exceeds this limit without terminating, LangGraph raises a `GraphRecursionError`.

```python
config = {
    "configurable": {"thread_id": "session-1"},
    "recursion_limit": 25  # Limit the graph execution to maximum 25 node transitions
}

try:
    graph.invoke(inputs, config)
except GraphRecursionError:
    print("Agent execution aborted: hit recursion ceiling.")
```

### B. Input/Output Guardrails (Llama Guard / Custom Filters)
*   **Prompt Injection:** Validate user inputs before they reach the model.
*   **Tool Arguments validation:** Never assume the LLM outputs correct types. Use Pydantic schemas to validate and parse args in the tool node.
*   **Command Whitelisting:** If your agent has a terminal/shell tool, do not allow arbitrary bash commands. Limit execution to a whitelist of commands, or require human approval for all write operations.

---

## 3. Designing Agentic Evaluations (Evals)

Traditional software testing checks if `f(x) == expected_output`. Because LLMs are non-deterministic, agent testing requires structured **Evaluations (Evals)**.

### A. Assertion-Based Evals
Write tests that assert facts about the output rather than exact string equality:

```python
def test_math_agent():
    result = math_graph.invoke({"messages": [HumanMessage(content="What is 4 times 8?")]})
    final_content = result["messages"][-1].content
    
    # 1. Assert numerical correctness
    assert "32" in final_content
    
    # 2. Assert structural validity
    assert len(result["messages"]) > 2  # Proves a tool was called
```

### B. Trajectory Evals
A trajectory eval checks the **path** the agent took to solve the task. 
*   *Did it call the `multiply` tool, or did it try to guess the answer?*
*   *Did it retry when a tool errored?*
*   *Did it hit the recursion limit?*

In LangGraph, you audit the trajectory by inspecting `state["messages"]` or the intermediate steps key.

### C. LLM-as-a-Judge
For qualitative tests (e.g., evaluating summary quality or tone), you can write an evaluation prompt where a stronger model (like GPT-4o) evaluates the output of your agent.

```python
from langchain_openai import ChatOpenAI
from langchain_core.prompts import PromptTemplate

eval_prompt = PromptTemplate.from_template("""
Evaluate the agent response based on correctness and helpfulness.
Agent Output: {agent_output}
Target Answer: {reference_answer}

Rate the output from 1 (completely wrong) to 5 (perfect). Respond with only the integer score.
""")

judge = ChatOpenAI(model="gpt-4o", temperature=0)
eval_chain = eval_prompt | judge

# Run evaluation
score = eval_chain.invoke({
    "agent_output": "The population of Tokyo is roughly 14 million people as of recent estimates.",
    "reference_answer": "14 million"
})
print("Judge Score:", score.content)  # Outputs: 5
```

---

## 4. Summary Exercises

1. **Exercise 1 (Implementing recursion limit handler):** Write a wrapper function around your agent invocation that catches `GraphRecursionError`, logs the failure, and returns a graceful fallback message to the user: `"I apologize, but this task is taking too long to complete. Please try simplifying your request."`
2. **Exercise 2 (LangSmith Walkthrough):** Sign up for a free LangSmith account, configure the environment variables, execute a cyclic tool-calling graph, and inspect the trace tree. Locate the exact node where a tool execution request occurred.

---

* [← back to Module 06](./06_multi_agent_architectures.md)*
* [← back to Roadmap](./00_roadmap.md)*
