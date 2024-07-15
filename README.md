# gedcb
Gossip-Enabled Distributed Circuit Breakers

## Development
### Setup
```console
kind create cluster
titl up
```

### Teardown
```console
kind delete cluster
```

## Notes
### Gossip-Enabled Distributed Circuit Breakers
- Phases of the protocol run in parallel.
- Phase A of the protocol is responsible for efficient and reliable sharing of information about the Distributed Circuit Breaker Node (DCBN) states of the clients, that are interacting with a given server, amongst each other.
- Phase B of the protocol keeps the set of clients participating in the gossip-based information dissemination up to date (and thus facilitates the discovery of client nodes which are interacting with the server in context).
#### Phase A: Client-Client Gossiping
- We probably want to set a minimum quorum that must be met for any majority vote to be considered valid.
- The gossipset should ideally include all the clients that are interacting with the same server instance.
- Every client maintains a gossip-set (GS) state that consists of opinions and ages (of corresponding opinions) about all the clients in the gossipset.
- An opinion of value 0 indicates that the DCBN is in the closed state and a value of 1 indicates that the DCBN is not in the closed state.
- The age of an opinion is the number of gossip cycles completed since the opinion originated from the source of truth.
- The self-opinion is always zero gossip cycles old as it is updated by the client itself in realtime.
- After every T1 unit of time (time period of gossiping), every client increments the age of all opinions (except the self-opinion).
- Then, the client gossips (sends its own state) to a randomly selected fixed-size subset of all the clients in its gossip-set.
- On receipt of a gossip message, a client updates its own GS state with opinions of a smaller age.
- If a node in the gossiping set of nodes goes down or becomes unreachable due to network partitioning, there will be no source of truth for the state of this node’s DCBN.
- An empirical threshold is fixed beyond which the age of an opinion is not allowed to increase to prevent incessant increase of the age of the state of the failed node.
- Once the threshold is reached the failed client is assumed to have crashed or be in a different network partition.
- Opinions of clients that have hit the age threshold should not be used when considering whether a majority of nodes are not in a closed state.
- The majority is determined based on the number of peers with young information not in the closed state.
- The entire set of five clients is considered for random selection of clients to send the gossip messages.
- This is done with the optimistic view that client #1 will become active again.

#### Phase B: Gossip-Set Revision
- Could be replaced by SWIM with lifeguard using an implementation such as [hashicorp/memberlist](https://github.com/hashicorp/memberlist).
- Since we are running in Kubernetes, we can rely on a headless service to determine the full set of peers and let Kubernetes deal with detecting and restarting failed clients.
- Server maintains set of clients.
- After every T2 time duration server increments the version, sends the list to a fixed subset of client nodes on the list and clears the list.
- The client's GS state are also annotated with the version number of the server state from which they were derived.
- When a client receives a set-revision message from the server, it updates its version number based on the message.
- The client discards the opinions about clients which are no longer in the new gossip-set suggested by the server.
- The client retains the opinions about the clients which are still present in the new gossip-set.
- The client adds optimistic opinions about the new clients found in the gossipset.
- These opinions about the new clients are assigned the highest age permissible for an opinion in the protocol (equal to the age limit).
- Due to this arrangement, on receiving subsequent gossip-messages, the client correctly accepts DCBN state information about these new clients which is encoded in the gossiped opinions (which are bound to have an age less than or equal to the highest permissible age).
- A gossip message having a GS state annotated with a lower version number is ignored.
- New clients essentially act as a traditional circuit breaker with the hard threshold.

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
- Maintains a buffer of changes to piggyback, when the buffer is too large for a single ping prefer newer information.
- Marking a node as faulty overrides any other suspect or alive messages about the node.
- Suspect and alive messages need to be distinguished so old messages don't override new state.
- Could choose ping target in round-robin fashion, but shuffle the membership list at each node.

### Lifeguard
- Slow message processing could lead to flapping healthy and faulty even with suspicion.
- Add a local health detector to the failure detection component.
- SWIM follows a fail-stop failure model.
- Local Health Aware Probe which makes SWIM’s protocol period and probe timeout adaptive.
- Local Health Aware Suspicion which makes SWIM’s suspicion timeout adaptive.
- Buddy System which prioritizes delivery of suspect messages to suspected members.
- Local Health Aware Suspicion uses a heuristic which starts the Suspicion timeout at a high value, and lowers it each time a suspect message is processed that indicates an independent suspicion of the same suspected member by some other member.
- The maximum that the Suspicion timeout starts at, the minimum that it drops to, and the number independent suspicions required to make it drop to the minimum (K) are configurable parameters of Lifeguard.
- Requiring K independent suspicions also reduces sensitivity to concurrent slow processing by other members, since the probability of multiple slow members falsely suspecting the same member reduces exponentially as K increases.

## Resources
- [Using Gossip Enabled Distributed Circuit Breaking for Improving Resiliency of Distributed Systems](https://ieeexplore.ieee.org/document/9779693)
- [SWIM: scalable weakly-consistent infection-style process group membership protocol](https://ieeexplore.ieee.org/document/1028914)
- [Lifeguard: Local Health Awareness for More Accurate Failure Detection](https://arxiv.org/pdf/1707.00788)