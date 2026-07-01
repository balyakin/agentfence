# Leak Lab

Leak-lab fixtures exercise AgentFence security invariants on Linux with `bubblewrap` and `gitleaks` installed.

Manual acceptance:

```sh
agentfence run generic --command ./leaklab/fake-agent.sh --task "probe"
agentfence diff latest
agentfence apply latest --branch
AF_INJECT_SECRET=1 agentfence run generic --command ./leaklab/fake-agent.sh --task "probe"
```
