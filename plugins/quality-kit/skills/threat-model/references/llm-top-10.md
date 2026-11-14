# OWASP Top 10 for LLM applications

The 2026 edition, published by the OWASP GenAI Security Project in September 2026: <https://genai.owasp.org/resource/owasp-genai-llm-top-10-2026/>. The numbers changed from the 2025 edition, so write each risk with its name.

Go through all ten. A risk applies when the feature gives it a way in.

| Risk | Applies when | Ask | Typical mitigations |
| --- | --- | --- | --- |
| **LLM01 Prompt Injection** | The model reads text someone else wrote: a user's message, a document, a web page, a tool's output | Can text in the input make the model do something other than its task? | Keep instructions apart from data, treat the answer as untrusted, give the model no more rights than the task needs, a person approves actions that matter |
| **LLM02 Sensitive Information Disclosure** | Personal, client or secret data goes into a prompt, or can come out in an answer, a log or a stored prompt | What does the provider receive and keep? Can one user's data reach another through the answer? | Send the least data, strip what the task does not need, an agreement with the provider on storage and training, access control on stored prompts |
| **LLM03 Excessive Agency** | The model's answer triggers an action: a tool call, an email, a write to a store | What is the worst the model can do when it is wrong or misled? | Few tools with narrow rights, the caller's rights and not the system's, a person confirms actions that cannot be undone |
| **LLM04 Supply Chain** | The feature depends on a model, an adapter, a dataset or a tool package from someone else | Who can change what you run, and would you notice? | Pin the model and version, take packages from known sources, a supplier agreement, a plan for when the provider changes or stops the model |
| **LLM05 Data and Model Poisoning** | You train or fine-tune, or the model reads a store that outsiders can write to | Can someone plant content that bends later answers? | Control who writes to what the model reads, review sources, keep the origin of each item |
| **LLM06 Unbounded Consumption** | Anyone can cause model calls, or one request can cause many or large ones | What does a flood of calls cost, in money and in service for others? | Limits on rate, input size and output size, a cap on spend, time-outs |
| **LLM07 Misinformation** | People or systems act on the answer | What happens when the answer is wrong and looks right? | A person reviews before it leaves, answers grounded in sources and showing them, a clear mark that the text is a draft |
| **LLM08 Hidden Context Exposure** | The prompt holds instructions or context the user is not meant to see | What does the system prompt reveal, and does anything rely on it staying secret? | No secrets or keys in the prompt, enforce rules in code and not in the prompt alone |
| **LLM09 Vector and Embedding Weaknesses** | The feature finds context by vector search, or keeps memory as embeddings | Can a search return what the asker may not see? Can planted content rank first? | Apply the asker's access rights to the search, separate stores for separate tenants, control what is indexed |
| **LLM10 Improper Output Handling** | The answer is put into a page, a query, a command or another system | What does the answer become downstream, and is it encoded for that place? | Treat the answer as user input: encode for HTML, use parameters for queries, validate against a schema |

A risk that does not apply still gets named, on the `Does not apply` line with its reason: "LLM05: no training, and only staff write to the help centre". The reader then sees that all ten were considered.

LLM rows count towards the budget of twelve threats. When a STRIDE row and an LLM row describe the same threat, keep the LLM row.
