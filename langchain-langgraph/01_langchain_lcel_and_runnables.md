# 01 — LangChain, LCEL, & Runnables

> **Goal:** Understand how LangChain Expression Language (LCEL) builds declarative pipelines of execution, and master the use of the Runnable protocol to stream, batch, and execute async chains.

---

## 1. What is LangChain?

**LangChain** is an open-source framework designed to simplify the development of applications powered by Large Language Models (LLMs). 

Rather than writing custom, boilerplate-heavy code to interact with model APIs, manage prompts, chain API calls, and parse outputs, LangChain provides a standardized interface and set of abstractions.

### The Core Value Proposition:
1. **Model Abstraction & Agnosticism:** You write code once, and you can easily swap between different model providers (e.g., OpenAI, Anthropic, Gemini, Cohere, or local models via Ollama/Llama.cpp) simply by changing the class instantiation.
2. **Standardized Interfaces (Runnables):** Every core component (prompts, models, parsers, retrievers) implements a unified protocol, allowing them to be composed into complex execution pipelines (chains).
3. **Ecosystem & Integrations:** LangChain features thousands of pre-built integrations for data loaders, vector databases, search tools, API connectors, and execution environments.

### The LangChain Ecosystem Packages:
To maintain a clean and lightweight codebase, LangChain is split into several modular packages:

*   **`langchain-core`**: The absolute foundation. It contains the base classes and abstractions (e.g., `BaseChatModel`, `BasePromptTemplate`, `BaseOutputParser`), the `Runnable` protocol, and LCEL. It has minimal dependencies.
*   **`langchain-community`**: Third-party integrations maintained by the community (e.g., vector database connectors like Pinecone/Chroma, search tools, document loaders).
*   **`langchain`**: Contains cognitive architecture logic, including chains, agent executors, and retrieval algorithms that form the core application layouts.
*   **Partner Packages** (e.g., `langchain-openai`, `langchain-anthropic`): Specialized packages containing direct, optimized integrations for specific model providers.
*   **`langgraph`**: The orchestration framework used for building cyclic, stateful multi-agent systems (the focus of Modules 03–07).

---


## 2. Setup & Environment

To work with LangChain and LangGraph, install the core packages and a model provider. In this tutorial, we will use `langchain-openai` or `langchain-anthropic` as examples.

```bash
pip install langchain langchain-core langchain-openai langchain-anthropic
```

Ensure your environment variables are configured:
```bash
# For OpenAI
export OPENAI_API_KEY="your-api-key"
# For Anthropic
export ANTHROPIC_API_KEY="your-api-key"
```

---

## 3. What is LCEL (LangChain Expression Language)?

LCEL is a declarative way to compose chains. It uses the pipe operator (`|`) to connect different components together. Under the hood, the pipe operator overrides the `__or__` method in Python to build a `RunnableSequence`.

### The Core Chain Pattern
A standard chain takes a user input, formats it into a prompt, passes it to the model, and parses the response:

```python
from langchain_core.prompts import ChatPromptTemplate
from langchain_core.output_parsers import StrOutputParser
from langchain_openai import ChatOpenAI

# 1. Initialize components
prompt = ChatPromptTemplate.from_messages([
    ("system", "You are a senior software engineer. Answer the user's question concisely."),
    ("user", "{question}")
])
model = ChatOpenAI(model="gpt-4o-mini", temperature=0.2)
parser = StrOutputParser()

# 2. Compose the chain using LCEL
chain = prompt | model | parser

# 3. Execute the chain
response = chain.invoke({"question": "What is the difference between concurrency and parallelism?"})
print(response)
```

---

## 4. The Runnable Protocol

Every component in LCEL implements the **Runnable** interface. This ensures a consistent API across prompts, models, retrievers, parsers, and custom functions.

### Core Runnable Methods

| Method | Type | Description |
| :--- | :--- | :--- |
| `invoke(input)` | Sync | Invokes the runnable on a single input. |
| `ainvoke(input)` | Async | Invokes the runnable asynchronously. |
| `stream(input)` | Sync | Streams back chunks of the output as they are generated. |
| `astream(input)` | Async | Streams back chunks of the output asynchronously. |
| `batch(inputs)` | Sync | Runs the runnable on a list of inputs in parallel. |
| `abatch(inputs)` | Async | Runs the runnable on a list of inputs in parallel asynchronously. |

### Example: Streaming Output
In agentic workflows, streaming is essential for UX to reduce perceived latency:

```python
import asyncio

async def run_streaming():
    # Asynchronously stream the output
    async for chunk in chain.astream({"question": "Write a 50-word summary of Docker."}):
        print(chunk, end="", flush=True)

asyncio.run(run_streaming())
```

---

## 5. Advanced Runnables: Parallelism & Data Flow

When building complex agent pipelines, you often need to route data, run tasks in parallel, or modify state mid-chain.

### A. `RunnablePassthrough`
Passes inputs unchanged or adds extra keys to the input dictionary.

```python
from langchain_core.runnables import RunnablePassthrough

# Format the input dictionary before passing to the prompt
formatted_chain = (
    {"question": RunnablePassthrough()} 
    | prompt 
    | model 
    | parser
)

# You can pass the string directly now
response = formatted_chain.invoke("Why is Python async single-threaded?")
```

### B. `RunnableParallel`
Runs multiple chains in parallel on the same input. Useful for performing independent sub-tasks before merging results.

```python
from langchain_core.runnables import RunnableParallel

# Define two parallel analysis tasks
analysis_chain = RunnableParallel(
    pros=ChatPromptTemplate.from_template("What are the pros of: {tech}?") | model | parser,
    cons=ChatPromptTemplate.from_template("What are the cons of: {tech}?") | model | parser
)

result = analysis_chain.invoke({"tech": "Kubernetes"})
print("PROS:\n", result["pros"])
print("\nCONS:\n", result["cons"])
```

### C. `RunnableLambda`
Wraps any custom Python function in a Runnable, allowing you to execute arbitrary logic (logging, database calls, string manipulation) inside an LCEL pipeline.

```python
from langchain_core.runnables import RunnableLambda

def count_words(text: str) -> int:
    return len(text.split())

# Wrap function in RunnableLambda
word_counter = RunnableLambda(count_words)

# Compose with the main chain
full_chain = chain | word_counter
word_count = full_chain.invoke({"question": "Name the capital of France."})
print(f"Response length: {word_count} words")
```

---

## 6. Summary Exercises

1. **Exercise 1 (Sync vs Async):** Convert the parallel chain example to run asynchronously using `.abatch()` or `.ainvoke()`. Measure the time difference between serial execution and parallel execution.
2. **Exercise 2 (Data Enrichment):** Create a chain that takes a database ID, uses a `RunnableLambda` to fetch a user's record (mocked), formats that record into a prompt, and uses an LLM to generate a personalized email.

---

* [next → 02_tool_calling_and_structured_outputs.md](./02_tool_calling_and_structured_outputs.md)*
* [← back to Roadmap](./00_roadmap.md)*
