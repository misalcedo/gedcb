# gedcb
Gossip-Enabled Distributed Circuit Breakers

## Notes
### SWIM
### Failure Detector
- A node `i` pings a random node `j`.
- Node `j` responds with an ack to node `i`.
- If node `i` does not receive an ack from node `j`, node `i` asks `k` other nodes to ping `j`.
- Node `i` could fail to receive the ack from node`j` for multiple reasons, including: node `j` has crashed, node `j` is slow to respond, the ack message was lost or delayed by the network.
- Node `i` must receive at least 1 ack from either node `j` or one of the `k` nodes.
- After the gossip period `T`, node `i` treats node `j` as failed in its local membership list if no ack is received.
- The gossip period `T` must be at least 3 times the round-trip time `t`.
- Node `i`'s suspicion of node `j` failing is passed on to the Dissemination Component.
- `k` tunes the probability of false positives expected by the application.
- `T` can be derived from an application-specified expected detection time. 

#### Dissemination Component - Multicast
- Multicast to all members.
- For a process to join the group, it would need to know at least one contact member in the group.
- In the absence of such infrastructure, join messages could be broadcast, and group members hearing it can probabilistically decide (by tossing a coin) whether to reply to it.
- Alternatively, to avoid multiple member replies, a static coordinator could be maintained within the group for the purpose of handling group join requests.
- Discovery and resolution of multiple coordinators can be done over time through the Dissemination Component.

#### Dissemination Component - Gossip
- Piggybacks membership updates on the ping and ack messages sent by the failure detector protocol.
- Node `i`, upon detecting node `j` as failed marks node `j` as suspected in its membership list and is disseminated to the group.
- After a pre-specified time-out, the suspected node `j` is marked as faulty and is disseminated to the group.
- If the suspected node `j` responds to a ping request before this time-out expires, information about this is disseminated to the group as an “alive” message.
- The pre-specified time-out thus effectively trades off an increase in failure detection time for a reduction in frequency of false failure detections.

## Resources
- [Using Gossip Enabled Distributed Circuit Breaking for Improving Resiliency of Distributed Systems](https://ieeexplore.ieee.org/document/9779693)
- [SWIM: scalable weakly-consistent infection-style process group membership protocol](https://ieeexplore.ieee.org/document/1028914)
- [Lifeguard: Local Health Awareness for More Accurate Failure Detection](https://ieeexplore.ieee.org/document/8416202)