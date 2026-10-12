This directory contains the spec I gave to Sonnet 5 to do a lot of the ground work. It did a decent job.
I've kept the spec and diagrams (I had Kimi generate them) here for visibilities sake. 


## Motivation
I had an idea for using two agents to debate over a topic and produce a verdict. My first implementation was a python based
MCP server you connected a harness to. I also tried this in go.

This worked, but I didn't like the idea of having to start the server each time, and connecting
the harness afterwards before I could use this feature.

I attended GopherconUK 2026, and got particularly excited about the [Kronk](https://github.com/ardanlabs/kronk) package: a go API that provided local inference with LLMs, 
with the intent to remove the need for a model server (or mcp server).

I wanted to experiment with using a spec document to guide an LLM on autopilot, rather than traditional
prompting in a harness, to move from a MCP server architecture to a TUI app. I had a spec document and had Kimi K3 refine it. The spec details the requirements, constraints, and UX design choices I wanted. 
I sent Sonnet 5 on the implementation.

The result worked... but was quite bloated. I was happy with the general direction but saw a lot of room for 
improvement in terms of design, maintainability, and code quality (you can see so in the commit history).


