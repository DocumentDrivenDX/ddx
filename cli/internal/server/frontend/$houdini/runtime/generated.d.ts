import type { Record } from "./public/record";
import { NodeInfo$result, NodeInfo$input } from "../artifacts/NodeInfo";
import { NodeInfoStore } from "../plugins/houdini-svelte/stores/NodeInfo";
import { TestTypename$result, TestTypename$input } from "../artifacts/TestTypename";
import { TestTypenameStore } from "../plugins/houdini-svelte/stores/TestTypename";

export declare type CacheTypeDef = {
    types: {
        PageInfo: {
            idFields: never;
            fields: {
                hasNextPage: {
                    type: boolean;
                    args: never;
                };
                hasPreviousPage: {
                    type: boolean;
                    args: never;
                };
                startCursor: {
                    type: string | null;
                    args: never;
                };
                endCursor: {
                    type: string | null;
                    args: never;
                };
            };
            fragments: [];
        };
        NodeInfo: {
            idFields: {
                id: string;
            };
            fields: {
                id: {
                    type: string;
                    args: never;
                };
                name: {
                    type: string;
                    args: never;
                };
                startedAt: {
                    type: any;
                    args: never;
                };
                lastSeen: {
                    type: any;
                    args: never;
                };
            };
            fragments: [];
        };
        Project: {
            idFields: {
                id: string;
            };
            fields: {
                id: {
                    type: string;
                    args: never;
                };
                name: {
                    type: string;
                    args: never;
                };
                path: {
                    type: string;
                    args: never;
                };
                gitRemote: {
                    type: string | null;
                    args: never;
                };
                registeredAt: {
                    type: any;
                    args: never;
                };
                lastSeen: {
                    type: any;
                    args: never;
                };
                unreachable: {
                    type: boolean | null;
                    args: never;
                };
                tombstonedAt: {
                    type: any | null;
                    args: never;
                };
            };
            fragments: [];
        };
        ProjectEdge: {
            idFields: never;
            fields: {
                node: {
                    type: Record<CacheTypeDef, "Project">;
                    args: never;
                };
                cursor: {
                    type: string;
                    args: never;
                };
            };
            fragments: [];
        };
        ProjectConnection: {
            idFields: never;
            fields: {
                edges: {
                    type: (Record<CacheTypeDef, "ProjectEdge">)[];
                    args: never;
                };
                pageInfo: {
                    type: Record<CacheTypeDef, "PageInfo">;
                    args: never;
                };
                totalCount: {
                    type: number;
                    args: never;
                };
            };
            fragments: [];
        };
        Bead: {
            idFields: {
                id: string;
            };
            fields: {
                id: {
                    type: string;
                    args: never;
                };
                title: {
                    type: string;
                    args: never;
                };
                status: {
                    type: string;
                    args: never;
                };
                priority: {
                    type: number;
                    args: never;
                };
                issueType: {
                    type: string;
                    args: never;
                };
                owner: {
                    type: string | null;
                    args: never;
                };
                createdAt: {
                    type: any;
                    args: never;
                };
                createdBy: {
                    type: string | null;
                    args: never;
                };
                updatedAt: {
                    type: any;
                    args: never;
                };
                labels: {
                    type: (string)[] | null;
                    args: never;
                };
                parent: {
                    type: string | null;
                    args: never;
                };
                description: {
                    type: string | null;
                    args: never;
                };
                acceptance: {
                    type: string | null;
                    args: never;
                };
                notes: {
                    type: string | null;
                    args: never;
                };
                dependencies: {
                    type: (Record<CacheTypeDef, "Dependency">)[] | null;
                    args: never;
                };
            };
            fragments: [];
        };
        Dependency: {
            idFields: never;
            fields: {
                issueId: {
                    type: string;
                    args: never;
                };
                dependsOnId: {
                    type: string;
                    args: never;
                };
                type: {
                    type: string;
                    args: never;
                };
                createdAt: {
                    type: string | null;
                    args: never;
                };
                createdBy: {
                    type: string | null;
                    args: never;
                };
                metadata: {
                    type: string | null;
                    args: never;
                };
            };
            fragments: [];
        };
        BeadEdge: {
            idFields: never;
            fields: {
                node: {
                    type: Record<CacheTypeDef, "Bead">;
                    args: never;
                };
                cursor: {
                    type: string;
                    args: never;
                };
            };
            fragments: [];
        };
        BeadConnection: {
            idFields: never;
            fields: {
                edges: {
                    type: (Record<CacheTypeDef, "BeadEdge">)[];
                    args: never;
                };
                pageInfo: {
                    type: Record<CacheTypeDef, "PageInfo">;
                    args: never;
                };
                totalCount: {
                    type: number;
                    args: never;
                };
            };
            fragments: [];
        };
        BeadStatusCounts: {
            idFields: never;
            fields: {
                open: {
                    type: number;
                    args: never;
                };
                closed: {
                    type: number;
                    args: never;
                };
                blocked: {
                    type: number;
                    args: never;
                };
                ready: {
                    type: number;
                    args: never;
                };
                total: {
                    type: number;
                    args: never;
                };
            };
            fragments: [];
        };
        Document: {
            idFields: {
                id: string;
            };
            fields: {
                id: {
                    type: string;
                    args: never;
                };
                path: {
                    type: string;
                    args: never;
                };
                title: {
                    type: string;
                    args: never;
                };
                dependsOn: {
                    type: (string)[];
                    args: never;
                };
                inputs: {
                    type: (string)[];
                    args: never;
                };
                dependents: {
                    type: (string)[];
                    args: never;
                };
                review: {
                    type: Record<CacheTypeDef, "DocumentReview"> | null;
                    args: never;
                };
                parkingLot: {
                    type: boolean;
                    args: never;
                };
                prompt: {
                    type: string | null;
                    args: never;
                };
                execDef: {
                    type: Record<CacheTypeDef, "DocumentExecDef"> | null;
                    args: never;
                };
            };
            fragments: [];
        };
        DocumentReview: {
            idFields: never;
            fields: {
                selfHash: {
                    type: string;
                    args: never;
                };
                deps: {
                    type: any;
                    args: never;
                };
                reviewedAt: {
                    type: string;
                    args: never;
                };
            };
            fragments: [];
        };
        DocumentExecDef: {
            idFields: {
                id: string;
            };
            fields: {
                id: {
                    type: string | null;
                    args: never;
                };
                artifactIds: {
                    type: (string)[] | null;
                    args: never;
                };
                executor: {
                    type: Record<CacheTypeDef, "ExecutorSpec"> | null;
                    args: never;
                };
                result: {
                    type: Record<CacheTypeDef, "ResultSpec"> | null;
                    args: never;
                };
                evaluation: {
                    type: Record<CacheTypeDef, "Evaluation"> | null;
                    args: never;
                };
                active: {
                    type: boolean | null;
                    args: never;
                };
                required: {
                    type: boolean | null;
                    args: never;
                };
                graphSource: {
                    type: boolean | null;
                    args: never;
                };
                createdAt: {
                    type: any | null;
                    args: never;
                };
            };
            fragments: [];
        };
        ExecutorSpec: {
            idFields: never;
            fields: {
                kind: {
                    type: string;
                    args: never;
                };
                command: {
                    type: (string)[] | null;
                    args: never;
                };
                cwd: {
                    type: string | null;
                    args: never;
                };
                env: {
                    type: any | null;
                    args: never;
                };
                timeoutMs: {
                    type: number | null;
                    args: never;
                };
            };
            fragments: [];
        };
        ResultSpec: {
            idFields: never;
            fields: {
                metric: {
                    type: Record<CacheTypeDef, "MetricResultSpec"> | null;
                    args: never;
                };
            };
            fragments: [];
        };
        MetricResultSpec: {
            idFields: never;
            fields: {
                unit: {
                    type: string | null;
                    args: never;
                };
                valuePath: {
                    type: string | null;
                    args: never;
                };
                samplesPath: {
                    type: string | null;
                    args: never;
                };
            };
            fragments: [];
        };
        Evaluation: {
            idFields: never;
            fields: {
                comparison: {
                    type: string | null;
                    args: never;
                };
                thresholds: {
                    type: Record<CacheTypeDef, "Thresholds"> | null;
                    args: never;
                };
            };
            fragments: [];
        };
        Thresholds: {
            idFields: never;
            fields: {
                warnMs: {
                    type: number | null;
                    args: never;
                };
                ratchetMs: {
                    type: number | null;
                    args: never;
                };
            };
            fragments: [];
        };
        DocumentEdge: {
            idFields: never;
            fields: {
                node: {
                    type: Record<CacheTypeDef, "Document">;
                    args: never;
                };
                cursor: {
                    type: string;
                    args: never;
                };
            };
            fragments: [];
        };
        DocumentConnection: {
            idFields: never;
            fields: {
                edges: {
                    type: (Record<CacheTypeDef, "DocumentEdge">)[];
                    args: never;
                };
                pageInfo: {
                    type: Record<CacheTypeDef, "PageInfo">;
                    args: never;
                };
                totalCount: {
                    type: number;
                    args: never;
                };
            };
            fragments: [];
        };
        DocGraph: {
            idFields: never;
            fields: {
                rootDir: {
                    type: string;
                    args: never;
                };
                documents: {
                    type: (Record<CacheTypeDef, "Document">)[];
                    args: never;
                };
                pathToId: {
                    type: any;
                    args: never;
                };
                dependents: {
                    type: any;
                    args: never;
                };
                warnings: {
                    type: (string)[];
                    args: never;
                };
            };
            fragments: [];
        };
        StaleReason: {
            idFields: {
                id: string;
            };
            fields: {
                id: {
                    type: string;
                    args: never;
                };
                path: {
                    type: string;
                    args: never;
                };
                reasons: {
                    type: (string)[];
                    args: never;
                };
            };
            fragments: [];
        };
        SearchResult: {
            idFields: never;
            fields: {
                path: {
                    type: string;
                    args: never;
                };
                type: {
                    type: string;
                    args: never;
                };
                name: {
                    type: string;
                    args: never;
                };
                snippet: {
                    type: string | null;
                    args: never;
                };
            };
            fragments: [];
        };
        SearchResultEdge: {
            idFields: never;
            fields: {
                node: {
                    type: Record<CacheTypeDef, "SearchResult">;
                    args: never;
                };
                cursor: {
                    type: string;
                    args: never;
                };
            };
            fragments: [];
        };
        SearchResultConnection: {
            idFields: never;
            fields: {
                edges: {
                    type: (Record<CacheTypeDef, "SearchResultEdge">)[];
                    args: never;
                };
                pageInfo: {
                    type: Record<CacheTypeDef, "PageInfo">;
                    args: never;
                };
                totalCount: {
                    type: number;
                    args: never;
                };
            };
            fragments: [];
        };
        Commit: {
            idFields: never;
            fields: {
                sha: {
                    type: string;
                    args: never;
                };
                shortSha: {
                    type: string;
                    args: never;
                };
                author: {
                    type: string;
                    args: never;
                };
                date: {
                    type: any;
                    args: never;
                };
                subject: {
                    type: string;
                    args: never;
                };
                body: {
                    type: string | null;
                    args: never;
                };
                beadRefs: {
                    type: (string)[] | null;
                    args: never;
                };
            };
            fragments: [];
        };
        CommitEdge: {
            idFields: never;
            fields: {
                node: {
                    type: Record<CacheTypeDef, "Commit">;
                    args: never;
                };
                cursor: {
                    type: string;
                    args: never;
                };
            };
            fragments: [];
        };
        CommitConnection: {
            idFields: never;
            fields: {
                edges: {
                    type: (Record<CacheTypeDef, "CommitEdge">)[];
                    args: never;
                };
                pageInfo: {
                    type: Record<CacheTypeDef, "PageInfo">;
                    args: never;
                };
                totalCount: {
                    type: number;
                    args: never;
                };
            };
            fragments: [];
        };
        Worker: {
            idFields: {
                id: string;
            };
            fields: {
                id: {
                    type: string;
                    args: never;
                };
                kind: {
                    type: string;
                    args: never;
                };
                state: {
                    type: string;
                    args: never;
                };
                status: {
                    type: string | null;
                    args: never;
                };
                projectRoot: {
                    type: string;
                    args: never;
                };
                harness: {
                    type: string | null;
                    args: never;
                };
                provider: {
                    type: string | null;
                    args: never;
                };
                model: {
                    type: string | null;
                    args: never;
                };
                effort: {
                    type: string | null;
                    args: never;
                };
                once: {
                    type: boolean | null;
                    args: never;
                };
                pollInterval: {
                    type: string | null;
                    args: never;
                };
                startedAt: {
                    type: any | null;
                    args: never;
                };
                finishedAt: {
                    type: any | null;
                    args: never;
                };
                error: {
                    type: string | null;
                    args: never;
                };
                stdoutPath: {
                    type: string | null;
                    args: never;
                };
                specPath: {
                    type: string | null;
                    args: never;
                };
                attempts: {
                    type: number | null;
                    args: never;
                };
                successes: {
                    type: number | null;
                    args: never;
                };
                failures: {
                    type: number | null;
                    args: never;
                };
                currentBead: {
                    type: string | null;
                    args: never;
                };
                lastError: {
                    type: string | null;
                    args: never;
                };
                lastResult: {
                    type: Record<CacheTypeDef, "WorkerExecutionResult"> | null;
                    args: never;
                };
                currentAttempt: {
                    type: Record<CacheTypeDef, "CurrentAttemptInfo"> | null;
                    args: never;
                };
                recentPhases: {
                    type: (Record<CacheTypeDef, "PhaseTransition">)[] | null;
                    args: never;
                };
                lastAttempt: {
                    type: Record<CacheTypeDef, "LastAttemptInfo"> | null;
                    args: never;
                };
                landSummary: {
                    type: Record<CacheTypeDef, "CoordinatorMetrics"> | null;
                    args: never;
                };
            };
            fragments: [];
        };
        CurrentAttemptInfo: {
            idFields: never;
            fields: {
                attemptId: {
                    type: string;
                    args: never;
                };
                beadId: {
                    type: string;
                    args: never;
                };
                beadTitle: {
                    type: string | null;
                    args: never;
                };
                harness: {
                    type: string | null;
                    args: never;
                };
                model: {
                    type: string | null;
                    args: never;
                };
                profile: {
                    type: string | null;
                    args: never;
                };
                phase: {
                    type: string;
                    args: never;
                };
                phaseSeq: {
                    type: number;
                    args: never;
                };
                startedAt: {
                    type: any;
                    args: never;
                };
                elapsedMs: {
                    type: number;
                    args: never;
                };
            };
            fragments: [];
        };
        PhaseTransition: {
            idFields: never;
            fields: {
                phase: {
                    type: string;
                    args: never;
                };
                ts: {
                    type: any;
                    args: never;
                };
                phaseSeq: {
                    type: number;
                    args: never;
                };
            };
            fragments: [];
        };
        LastAttemptInfo: {
            idFields: never;
            fields: {
                attemptId: {
                    type: string;
                    args: never;
                };
                beadId: {
                    type: string;
                    args: never;
                };
                phase: {
                    type: string;
                    args: never;
                };
                startedAt: {
                    type: any;
                    args: never;
                };
                endedAt: {
                    type: any;
                    args: never;
                };
                elapsedMs: {
                    type: number;
                    args: never;
                };
            };
            fragments: [];
        };
        WorkerExecutionResult: {
            idFields: never;
            fields: {
                beadId: {
                    type: string | null;
                    args: never;
                };
                attemptId: {
                    type: string | null;
                    args: never;
                };
                workerId: {
                    type: string | null;
                    args: never;
                };
                harness: {
                    type: string | null;
                    args: never;
                };
                provider: {
                    type: string | null;
                    args: never;
                };
                model: {
                    type: string | null;
                    args: never;
                };
                status: {
                    type: string | null;
                    args: never;
                };
                detail: {
                    type: string | null;
                    args: never;
                };
                sessionId: {
                    type: string | null;
                    args: never;
                };
                baseRev: {
                    type: string | null;
                    args: never;
                };
                resultRev: {
                    type: string | null;
                    args: never;
                };
                retryAfter: {
                    type: string | null;
                    args: never;
                };
            };
            fragments: [];
        };
        WorkerEdge: {
            idFields: never;
            fields: {
                node: {
                    type: Record<CacheTypeDef, "Worker">;
                    args: never;
                };
                cursor: {
                    type: string;
                    args: never;
                };
            };
            fragments: [];
        };
        WorkerConnection: {
            idFields: never;
            fields: {
                edges: {
                    type: (Record<CacheTypeDef, "WorkerEdge">)[];
                    args: never;
                };
                pageInfo: {
                    type: Record<CacheTypeDef, "PageInfo">;
                    args: never;
                };
                totalCount: {
                    type: number;
                    args: never;
                };
            };
            fragments: [];
        };
        WorkerLog: {
            idFields: never;
            fields: {
                stdout: {
                    type: string;
                    args: never;
                };
                stderr: {
                    type: string;
                    args: never;
                };
            };
            fragments: [];
        };
        AgentSession: {
            idFields: {
                id: string;
            };
            fields: {
                id: {
                    type: string;
                    args: never;
                };
                projectId: {
                    type: string;
                    args: never;
                };
                beadId: {
                    type: string | null;
                    args: never;
                };
                harness: {
                    type: string;
                    args: never;
                };
                model: {
                    type: string;
                    args: never;
                };
                effort: {
                    type: string;
                    args: never;
                };
                status: {
                    type: string;
                    args: never;
                };
                startedAt: {
                    type: any;
                    args: never;
                };
                endedAt: {
                    type: any | null;
                    args: never;
                };
                stdoutPath: {
                    type: string | null;
                    args: never;
                };
                stderrPath: {
                    type: string | null;
                    args: never;
                };
                durationMs: {
                    type: number;
                    args: never;
                };
                cost: {
                    type: number | null;
                    args: never;
                };
                tokens: {
                    type: Record<CacheTypeDef, "TokenUsage"> | null;
                    args: never;
                };
                outcome: {
                    type: string | null;
                    args: never;
                };
                detail: {
                    type: string | null;
                    args: never;
                };
                resultRev: {
                    type: string | null;
                    args: never;
                };
                baseRev: {
                    type: string | null;
                    args: never;
                };
            };
            fragments: [];
        };
        TokenUsage: {
            idFields: never;
            fields: {
                prompt: {
                    type: number | null;
                    args: never;
                };
                completion: {
                    type: number | null;
                    args: never;
                };
                total: {
                    type: number | null;
                    args: never;
                };
                cached: {
                    type: number | null;
                    args: never;
                };
            };
            fragments: [];
        };
        AgentSessionEdge: {
            idFields: never;
            fields: {
                node: {
                    type: Record<CacheTypeDef, "AgentSession">;
                    args: never;
                };
                cursor: {
                    type: string;
                    args: never;
                };
            };
            fragments: [];
        };
        AgentSessionConnection: {
            idFields: never;
            fields: {
                edges: {
                    type: (Record<CacheTypeDef, "AgentSessionEdge">)[];
                    args: never;
                };
                pageInfo: {
                    type: Record<CacheTypeDef, "PageInfo">;
                    args: never;
                };
                totalCount: {
                    type: number;
                    args: never;
                };
            };
            fragments: [];
        };
        Persona: {
            idFields: {
                id: string;
            };
            fields: {
                id: {
                    type: string;
                    args: never;
                };
                name: {
                    type: string;
                    args: never;
                };
                roles: {
                    type: (string)[];
                    args: never;
                };
                description: {
                    type: string;
                    args: never;
                };
                tags: {
                    type: (string)[];
                    args: never;
                };
                content: {
                    type: string;
                    args: never;
                };
                filePath: {
                    type: string | null;
                    args: never;
                };
                modTime: {
                    type: any | null;
                    args: never;
                };
            };
            fragments: [];
        };
        PersonaEdge: {
            idFields: never;
            fields: {
                node: {
                    type: Record<CacheTypeDef, "Persona">;
                    args: never;
                };
                cursor: {
                    type: string;
                    args: never;
                };
            };
            fragments: [];
        };
        PersonaConnection: {
            idFields: never;
            fields: {
                edges: {
                    type: (Record<CacheTypeDef, "PersonaEdge">)[];
                    args: never;
                };
                pageInfo: {
                    type: Record<CacheTypeDef, "PageInfo">;
                    args: never;
                };
                totalCount: {
                    type: number;
                    args: never;
                };
            };
            fragments: [];
        };
        ExecutionDefinition: {
            idFields: {
                id: string;
            };
            fields: {
                id: {
                    type: string;
                    args: never;
                };
                artifactIds: {
                    type: (string)[];
                    args: never;
                };
                executor: {
                    type: Record<CacheTypeDef, "ExecutorSpec">;
                    args: never;
                };
                result: {
                    type: Record<CacheTypeDef, "ResultSpec"> | null;
                    args: never;
                };
                evaluation: {
                    type: Record<CacheTypeDef, "Evaluation"> | null;
                    args: never;
                };
                active: {
                    type: boolean;
                    args: never;
                };
                required: {
                    type: boolean | null;
                    args: never;
                };
                graphSource: {
                    type: boolean | null;
                    args: never;
                };
                createdAt: {
                    type: any;
                    args: never;
                };
            };
            fragments: [];
        };
        ExecutionDefinitionEdge: {
            idFields: never;
            fields: {
                node: {
                    type: Record<CacheTypeDef, "ExecutionDefinition">;
                    args: never;
                };
                cursor: {
                    type: string;
                    args: never;
                };
            };
            fragments: [];
        };
        ExecutionDefinitionConnection: {
            idFields: never;
            fields: {
                edges: {
                    type: (Record<CacheTypeDef, "ExecutionDefinitionEdge">)[];
                    args: never;
                };
                pageInfo: {
                    type: Record<CacheTypeDef, "PageInfo">;
                    args: never;
                };
                totalCount: {
                    type: number;
                    args: never;
                };
            };
            fragments: [];
        };
        ExecutionRun: {
            idFields: {
                id: string;
            };
            fields: {
                id: {
                    type: string;
                    args: never;
                };
                definitionId: {
                    type: string;
                    args: never;
                };
                artifactIds: {
                    type: (string)[];
                    args: never;
                };
                startedAt: {
                    type: any;
                    args: never;
                };
                finishedAt: {
                    type: any;
                    args: never;
                };
                status: {
                    type: string;
                    args: never;
                };
                exitCode: {
                    type: number;
                    args: never;
                };
                mergeBlocking: {
                    type: boolean | null;
                    args: never;
                };
                agentSessionId: {
                    type: string | null;
                    args: never;
                };
                attachments: {
                    type: any | null;
                    args: never;
                };
                provenance: {
                    type: Record<CacheTypeDef, "Provenance"> | null;
                    args: never;
                };
                metric: {
                    type: Record<CacheTypeDef, "MetricObservation"> | null;
                    args: never;
                };
                stdout: {
                    type: string | null;
                    args: never;
                };
                stderr: {
                    type: string | null;
                    args: never;
                };
                value: {
                    type: number | null;
                    args: never;
                };
                unit: {
                    type: string | null;
                    args: never;
                };
                parsed: {
                    type: boolean | null;
                    args: never;
                };
            };
            fragments: [];
        };
        ExecutionRunEdge: {
            idFields: never;
            fields: {
                node: {
                    type: Record<CacheTypeDef, "ExecutionRun">;
                    args: never;
                };
                cursor: {
                    type: string;
                    args: never;
                };
            };
            fragments: [];
        };
        ExecutionRunConnection: {
            idFields: never;
            fields: {
                edges: {
                    type: (Record<CacheTypeDef, "ExecutionRunEdge">)[];
                    args: never;
                };
                pageInfo: {
                    type: Record<CacheTypeDef, "PageInfo">;
                    args: never;
                };
                totalCount: {
                    type: number;
                    args: never;
                };
            };
            fragments: [];
        };
        ExecutionRunLog: {
            idFields: never;
            fields: {
                stdout: {
                    type: string;
                    args: never;
                };
                stderr: {
                    type: string;
                    args: never;
                };
            };
            fragments: [];
        };
        Provenance: {
            idFields: never;
            fields: {
                actor: {
                    type: string | null;
                    args: never;
                };
                host: {
                    type: string | null;
                    args: never;
                };
                gitRev: {
                    type: string | null;
                    args: never;
                };
                ddxVersion: {
                    type: string | null;
                    args: never;
                };
            };
            fragments: [];
        };
        MetricObservation: {
            idFields: never;
            fields: {
                artifactId: {
                    type: string;
                    args: never;
                };
                definitionId: {
                    type: string;
                    args: never;
                };
                observedAt: {
                    type: any;
                    args: never;
                };
                status: {
                    type: string;
                    args: never;
                };
                value: {
                    type: number;
                    args: never;
                };
                unit: {
                    type: string | null;
                    args: never;
                };
                samples: {
                    type: (number)[] | null;
                    args: never;
                };
                comparison: {
                    type: Record<CacheTypeDef, "ComparisonResult"> | null;
                    args: never;
                };
            };
            fragments: [];
        };
        ComparisonResult: {
            idFields: never;
            fields: {
                baseline: {
                    type: number;
                    args: never;
                };
                delta: {
                    type: number;
                    args: never;
                };
                direction: {
                    type: string;
                    args: never;
                };
            };
            fragments: [];
        };
        LandOutcomeSummary: {
            idFields: never;
            fields: {
                ts: {
                    type: any;
                    args: never;
                };
                beadId: {
                    type: string | null;
                    args: never;
                };
                attemptId: {
                    type: string | null;
                    args: never;
                };
                outcome: {
                    type: string;
                    args: never;
                };
                durationMs: {
                    type: number;
                    args: never;
                };
                commitCount: {
                    type: number;
                    args: never;
                };
            };
            fragments: [];
        };
        CoordinatorMetrics: {
            idFields: never;
            fields: {
                landed: {
                    type: number;
                    args: never;
                };
                preserved: {
                    type: number;
                    args: never;
                };
                failed: {
                    type: number;
                    args: never;
                };
                pushFailed: {
                    type: number;
                    args: never;
                };
                totalDurationMs: {
                    type: number;
                    args: never;
                };
                totalCommits: {
                    type: number;
                    args: never;
                };
                preservedRatio: {
                    type: number;
                    args: never;
                };
                lastSubmissions: {
                    type: (Record<CacheTypeDef, "LandOutcomeSummary">)[] | null;
                    args: never;
                };
            };
            fragments: [];
        };
        CoordinatorMetricsEntry: {
            idFields: never;
            fields: {
                projectRoot: {
                    type: string;
                    args: never;
                };
                metrics: {
                    type: Record<CacheTypeDef, "CoordinatorMetrics">;
                    args: never;
                };
            };
            fragments: [];
        };
        HealthStatus: {
            idFields: never;
            fields: {
                status: {
                    type: string;
                    args: never;
                };
                startedAt: {
                    type: any;
                    args: never;
                };
            };
            fragments: [];
        };
        ReadyCheck: {
            idFields: never;
            fields: {
                name: {
                    type: string;
                    args: never;
                };
                status: {
                    type: string;
                    args: never;
                };
            };
            fragments: [];
        };
        ReadyStatus: {
            idFields: never;
            fields: {
                ready: {
                    type: boolean;
                    args: never;
                };
                checks: {
                    type: (Record<CacheTypeDef, "ReadyCheck">)[];
                    args: never;
                };
            };
            fragments: [];
        };
        Window: {
            idFields: never;
            fields: {
                since: {
                    type: any | null;
                    args: never;
                };
            };
            fragments: [];
        };
        SourceRef: {
            idFields: never;
            fields: {
                beadId: {
                    type: string | null;
                    args: never;
                };
                sessionId: {
                    type: string | null;
                    args: never;
                };
                nativeSessionId: {
                    type: string | null;
                    args: never;
                };
                nativeLogRef: {
                    type: string | null;
                    args: never;
                };
                closingCommitSha: {
                    type: string | null;
                    args: never;
                };
                timestamp: {
                    type: any | null;
                    args: never;
                };
                source: {
                    type: string | null;
                    args: never;
                };
            };
            fragments: [];
        };
        AggregateSummaryBeads: {
            idFields: never;
            fields: {
                total: {
                    type: number;
                    args: never;
                };
                open: {
                    type: number;
                    args: never;
                };
                inProgress: {
                    type: number;
                    args: never;
                };
                closed: {
                    type: number;
                    args: never;
                };
                reopened: {
                    type: number;
                    args: never;
                };
                knownCycleTime: {
                    type: number;
                    args: never;
                };
                unknownCycleTime: {
                    type: number;
                    args: never;
                };
                knownCost: {
                    type: number;
                    args: never;
                };
                estimatedCost: {
                    type: number;
                    args: never;
                };
                unknownCost: {
                    type: number;
                    args: never;
                };
            };
            fragments: [];
        };
        AggregateSummarySessions: {
            idFields: never;
            fields: {
                total: {
                    type: number;
                    args: never;
                };
                correlated: {
                    type: number;
                    args: never;
                };
                uncorrelated: {
                    type: number;
                    args: never;
                };
                inputTokens: {
                    type: number;
                    args: never;
                };
                outputTokens: {
                    type: number;
                    args: never;
                };
                totalTokens: {
                    type: number;
                    args: never;
                };
                knownCost: {
                    type: number;
                    args: never;
                };
                estimatedCost: {
                    type: number;
                    args: never;
                };
                unknownCost: {
                    type: number;
                    args: never;
                };
                costUsd: {
                    type: number;
                    args: never;
                };
            };
            fragments: [];
        };
        AggregateSummaryCost: {
            idFields: never;
            fields: {
                beads: {
                    type: number;
                    args: never;
                };
                features: {
                    type: number;
                    args: never;
                };
                knownCostUsd: {
                    type: number;
                    args: never;
                };
                estimatedCostUsd: {
                    type: number;
                    args: never;
                };
                unknownBeads: {
                    type: number;
                    args: never;
                };
            };
            fragments: [];
        };
        AggregateSummaryCycleTime: {
            idFields: never;
            fields: {
                knownCount: {
                    type: number;
                    args: never;
                };
                unknownCount: {
                    type: number;
                    args: never;
                };
                averageMs: {
                    type: number | null;
                    args: never;
                };
                minMs: {
                    type: number | null;
                    args: never;
                };
                maxMs: {
                    type: number | null;
                    args: never;
                };
            };
            fragments: [];
        };
        AggregateSummaryRework: {
            idFields: never;
            fields: {
                knownClosed: {
                    type: number;
                    args: never;
                };
                knownReopened: {
                    type: number;
                    args: never;
                };
                unknownCount: {
                    type: number;
                    args: never;
                };
                reopenRate: {
                    type: number;
                    args: never;
                };
                revisionCount: {
                    type: number;
                    args: never;
                };
            };
            fragments: [];
        };
        AggregateSummary: {
            idFields: never;
            fields: {
                beads: {
                    type: Record<CacheTypeDef, "AggregateSummaryBeads">;
                    args: never;
                };
                sessions: {
                    type: Record<CacheTypeDef, "AggregateSummarySessions">;
                    args: never;
                };
                cost: {
                    type: Record<CacheTypeDef, "AggregateSummaryCost">;
                    args: never;
                };
                cycleTime: {
                    type: Record<CacheTypeDef, "AggregateSummaryCycleTime">;
                    args: never;
                };
                rework: {
                    type: Record<CacheTypeDef, "AggregateSummaryRework">;
                    args: never;
                };
            };
            fragments: [];
        };
        CostSummary: {
            idFields: never;
            fields: {
                beads: {
                    type: number;
                    args: never;
                };
                features: {
                    type: number;
                    args: never;
                };
                knownCostUsd: {
                    type: number;
                    args: never;
                };
                estimatedCostUsd: {
                    type: number;
                    args: never;
                };
                unknownBeads: {
                    type: number;
                    args: never;
                };
            };
            fragments: [];
        };
        BeadCostRow: {
            idFields: never;
            fields: {
                beadId: {
                    type: string;
                    args: never;
                };
                title: {
                    type: string;
                    args: never;
                };
                specId: {
                    type: string | null;
                    args: never;
                };
                status: {
                    type: string;
                    args: never;
                };
                sessionIds: {
                    type: (string)[] | null;
                    args: never;
                };
                inputTokens: {
                    type: number;
                    args: never;
                };
                outputTokens: {
                    type: number;
                    args: never;
                };
                totalTokens: {
                    type: number;
                    args: never;
                };
                costState: {
                    type: string;
                    args: never;
                };
                costUsd: {
                    type: number | null;
                    args: never;
                };
                unknownSessions: {
                    type: number | null;
                    args: never;
                };
                provenance: {
                    type: (Record<CacheTypeDef, "SourceRef">)[] | null;
                    args: never;
                };
            };
            fragments: [];
        };
        FeatureCostRow: {
            idFields: never;
            fields: {
                specId: {
                    type: string;
                    args: never;
                };
                beadIds: {
                    type: (string)[] | null;
                    args: never;
                };
                inputTokens: {
                    type: number;
                    args: never;
                };
                outputTokens: {
                    type: number;
                    args: never;
                };
                totalTokens: {
                    type: number;
                    args: never;
                };
                costState: {
                    type: string;
                    args: never;
                };
                costUsd: {
                    type: number | null;
                    args: never;
                };
                unknownBeads: {
                    type: number | null;
                    args: never;
                };
            };
            fragments: [];
        };
        CostReport: {
            idFields: never;
            fields: {
                scope: {
                    type: string;
                    args: never;
                };
                window: {
                    type: Record<CacheTypeDef, "Window"> | null;
                    args: never;
                };
                beadId: {
                    type: string | null;
                    args: never;
                };
                featureId: {
                    type: string | null;
                    args: never;
                };
                beads: {
                    type: (Record<CacheTypeDef, "BeadCostRow">)[] | null;
                    args: never;
                };
                features: {
                    type: (Record<CacheTypeDef, "FeatureCostRow">)[] | null;
                    args: never;
                };
                summary: {
                    type: Record<CacheTypeDef, "CostSummary">;
                    args: never;
                };
            };
            fragments: [];
        };
        CycleTimeSummary: {
            idFields: never;
            fields: {
                knownCount: {
                    type: number;
                    args: never;
                };
                unknownCount: {
                    type: number;
                    args: never;
                };
                averageMs: {
                    type: number | null;
                    args: never;
                };
                minMs: {
                    type: number | null;
                    args: never;
                };
                maxMs: {
                    type: number | null;
                    args: never;
                };
            };
            fragments: [];
        };
        CycleTimeRow: {
            idFields: never;
            fields: {
                beadId: {
                    type: string;
                    args: never;
                };
                title: {
                    type: string;
                    args: never;
                };
                specId: {
                    type: string | null;
                    args: never;
                };
                status: {
                    type: string;
                    args: never;
                };
                createdAt: {
                    type: any;
                    args: never;
                };
                firstClosedAt: {
                    type: any | null;
                    args: never;
                };
                lastClosedAt: {
                    type: any | null;
                    args: never;
                };
                cycleTimeMs: {
                    type: number | null;
                    args: never;
                };
                reopenCount: {
                    type: number | null;
                    args: never;
                };
                revisionCount: {
                    type: number | null;
                    args: never;
                };
                timeInOpenMs: {
                    type: number | null;
                    args: never;
                };
                timeInProgressMs: {
                    type: number | null;
                    args: never;
                };
                timeInClosedMs: {
                    type: number | null;
                    args: never;
                };
                cycleState: {
                    type: string;
                    args: never;
                };
                provenance: {
                    type: (Record<CacheTypeDef, "SourceRef">)[] | null;
                    args: never;
                };
            };
            fragments: [];
        };
        CycleTimeReport: {
            idFields: never;
            fields: {
                scope: {
                    type: string;
                    args: never;
                };
                window: {
                    type: Record<CacheTypeDef, "Window"> | null;
                    args: never;
                };
                beads: {
                    type: (Record<CacheTypeDef, "CycleTimeRow">)[] | null;
                    args: never;
                };
                summary: {
                    type: Record<CacheTypeDef, "CycleTimeSummary">;
                    args: never;
                };
            };
            fragments: [];
        };
        ReworkSummary: {
            idFields: never;
            fields: {
                knownClosed: {
                    type: number;
                    args: never;
                };
                knownReopened: {
                    type: number;
                    args: never;
                };
                unknownCount: {
                    type: number;
                    args: never;
                };
                reopenRate: {
                    type: number;
                    args: never;
                };
                revisionCount: {
                    type: number;
                    args: never;
                };
            };
            fragments: [];
        };
        ReworkRow: {
            idFields: never;
            fields: {
                beadId: {
                    type: string;
                    args: never;
                };
                title: {
                    type: string;
                    args: never;
                };
                specId: {
                    type: string | null;
                    args: never;
                };
                status: {
                    type: string;
                    args: never;
                };
                firstClosedAt: {
                    type: any | null;
                    args: never;
                };
                reopened: {
                    type: boolean;
                    args: never;
                };
                reopenCount: {
                    type: number | null;
                    args: never;
                };
                revisionCount: {
                    type: number | null;
                    args: never;
                };
                reworkState: {
                    type: string;
                    args: never;
                };
                provenance: {
                    type: (Record<CacheTypeDef, "SourceRef">)[] | null;
                    args: never;
                };
            };
            fragments: [];
        };
        ReworkReport: {
            idFields: never;
            fields: {
                scope: {
                    type: string;
                    args: never;
                };
                window: {
                    type: Record<CacheTypeDef, "Window"> | null;
                    args: never;
                };
                beads: {
                    type: (Record<CacheTypeDef, "ReworkRow">)[] | null;
                    args: never;
                };
                summary: {
                    type: Record<CacheTypeDef, "ReworkSummary">;
                    args: never;
                };
            };
            fragments: [];
        };
        Provider: {
            idFields: {
                id: string;
            };
            fields: {
                id: {
                    type: string;
                    args: never;
                };
                name: {
                    type: string;
                    args: never;
                };
                kind: {
                    type: string;
                    args: never;
                };
                defaultModel: {
                    type: string | null;
                    args: never;
                };
                models: {
                    type: (string)[] | null;
                    args: never;
                };
                defaultEffort: {
                    type: string | null;
                    args: never;
                };
                effortLevels: {
                    type: (string)[] | null;
                    args: never;
                };
                timeoutMs: {
                    type: number | null;
                    args: never;
                };
                permissions: {
                    type: (string)[] | null;
                    args: never;
                };
            };
            fragments: [];
        };
        __ROOT__: {
            idFields: {};
            fields: {
                node: {
                    type: Record<CacheTypeDef, "NodeInfo"> | Record<CacheTypeDef, "Project"> | Record<CacheTypeDef, "Bead"> | Record<CacheTypeDef, "Document"> | Record<CacheTypeDef, "Worker"> | Record<CacheTypeDef, "AgentSession"> | Record<CacheTypeDef, "Persona"> | Record<CacheTypeDef, "ExecutionDefinition"> | Record<CacheTypeDef, "ExecutionRun"> | Record<CacheTypeDef, "Provider"> | null;
                    args: {
                        id: string;
                    };
                };
                nodeInfo: {
                    type: Record<CacheTypeDef, "NodeInfo">;
                    args: never;
                };
                projects: {
                    type: Record<CacheTypeDef, "ProjectConnection">;
                    args: {
                        first?: number | null | undefined;
                        after?: string | null | undefined;
                        last?: number | null | undefined;
                        before?: string | null | undefined;
                        includeUnreachable?: boolean | null | undefined;
                    };
                };
                beads: {
                    type: Record<CacheTypeDef, "BeadConnection">;
                    args: {
                        first?: number | null | undefined;
                        after?: string | null | undefined;
                        last?: number | null | undefined;
                        before?: string | null | undefined;
                        status?: string | null | undefined;
                        label?: string | null | undefined;
                        projectID?: string | null | undefined;
                    };
                };
                beadsByProject: {
                    type: Record<CacheTypeDef, "BeadConnection">;
                    args: {
                        projectID: string;
                        first?: number | null | undefined;
                        after?: string | null | undefined;
                        last?: number | null | undefined;
                        before?: string | null | undefined;
                        status?: string | null | undefined;
                        label?: string | null | undefined;
                    };
                };
                beadsReady: {
                    type: Record<CacheTypeDef, "BeadConnection">;
                    args: {
                        first?: number | null | undefined;
                        after?: string | null | undefined;
                        last?: number | null | undefined;
                        before?: string | null | undefined;
                    };
                };
                beadsBlocked: {
                    type: Record<CacheTypeDef, "BeadConnection">;
                    args: {
                        first?: number | null | undefined;
                        after?: string | null | undefined;
                        last?: number | null | undefined;
                        before?: string | null | undefined;
                    };
                };
                beadsStatus: {
                    type: Record<CacheTypeDef, "BeadStatusCounts">;
                    args: never;
                };
                beadDepTree: {
                    type: string;
                    args: {
                        beadID: string;
                    };
                };
                bead: {
                    type: Record<CacheTypeDef, "Bead"> | null;
                    args: {
                        id: string;
                    };
                };
                documents: {
                    type: Record<CacheTypeDef, "DocumentConnection">;
                    args: {
                        first?: number | null | undefined;
                        after?: string | null | undefined;
                        last?: number | null | undefined;
                        before?: string | null | undefined;
                        type?: string | null | undefined;
                    };
                };
                documentByPath: {
                    type: Record<CacheTypeDef, "Document"> | null;
                    args: {
                        path: string;
                    };
                };
                docGraph: {
                    type: Record<CacheTypeDef, "DocGraph">;
                    args: never;
                };
                docStale: {
                    type: (Record<CacheTypeDef, "StaleReason">)[];
                    args: never;
                };
                docDeps: {
                    type: (string)[];
                    args: {
                        documentID: string;
                    };
                };
                docDependents: {
                    type: (string)[];
                    args: {
                        documentID: string;
                    };
                };
                docHistory: {
                    type: Record<CacheTypeDef, "CommitConnection">;
                    args: {
                        documentID: string;
                        first?: number | null | undefined;
                        after?: string | null | undefined;
                        last?: number | null | undefined;
                        before?: string | null | undefined;
                    };
                };
                docDiff: {
                    type: string;
                    args: {
                        documentID: string;
                        ref?: string | null | undefined;
                    };
                };
                doc: {
                    type: Record<CacheTypeDef, "Document"> | null;
                    args: {
                        id: string;
                    };
                };
                search: {
                    type: Record<CacheTypeDef, "SearchResultConnection">;
                    args: {
                        query: string;
                        first?: number | null | undefined;
                        after?: string | null | undefined;
                        last?: number | null | undefined;
                        before?: string | null | undefined;
                    };
                };
                commits: {
                    type: Record<CacheTypeDef, "CommitConnection">;
                    args: {
                        projectID: string;
                        first?: number | null | undefined;
                        after?: string | null | undefined;
                        last?: number | null | undefined;
                        before?: string | null | undefined;
                        since?: string | null | undefined;
                        author?: string | null | undefined;
                    };
                };
                workers: {
                    type: Record<CacheTypeDef, "WorkerConnection">;
                    args: {
                        first?: number | null | undefined;
                        after?: string | null | undefined;
                        last?: number | null | undefined;
                        before?: string | null | undefined;
                    };
                };
                workersByProject: {
                    type: Record<CacheTypeDef, "WorkerConnection">;
                    args: {
                        projectID: string;
                        first?: number | null | undefined;
                        after?: string | null | undefined;
                        last?: number | null | undefined;
                        before?: string | null | undefined;
                    };
                };
                worker: {
                    type: Record<CacheTypeDef, "Worker"> | null;
                    args: {
                        id: string;
                    };
                };
                workerProgress: {
                    type: (Record<CacheTypeDef, "PhaseTransition">)[];
                    args: {
                        workerID: string;
                    };
                };
                workerLog: {
                    type: Record<CacheTypeDef, "WorkerLog">;
                    args: {
                        workerID: string;
                    };
                };
                workerPrompt: {
                    type: string;
                    args: {
                        workerID: string;
                    };
                };
                agentSessions: {
                    type: Record<CacheTypeDef, "AgentSessionConnection">;
                    args: {
                        first?: number | null | undefined;
                        after?: string | null | undefined;
                        last?: number | null | undefined;
                        before?: string | null | undefined;
                    };
                };
                agentSession: {
                    type: Record<CacheTypeDef, "AgentSession"> | null;
                    args: {
                        id: string;
                    };
                };
                personas: {
                    type: Record<CacheTypeDef, "PersonaConnection">;
                    args: {
                        first?: number | null | undefined;
                        after?: string | null | undefined;
                        last?: number | null | undefined;
                        before?: string | null | undefined;
                    };
                };
                persona: {
                    type: Record<CacheTypeDef, "Persona"> | null;
                    args: {
                        name: string;
                    };
                };
                personaByRole: {
                    type: Record<CacheTypeDef, "Persona"> | null;
                    args: {
                        role: string;
                    };
                };
                execDefinitions: {
                    type: Record<CacheTypeDef, "ExecutionDefinitionConnection">;
                    args: {
                        first?: number | null | undefined;
                        after?: string | null | undefined;
                        last?: number | null | undefined;
                        before?: string | null | undefined;
                        artifactID?: string | null | undefined;
                    };
                };
                execDefinition: {
                    type: Record<CacheTypeDef, "ExecutionDefinition"> | null;
                    args: {
                        id: string;
                    };
                };
                execRuns: {
                    type: Record<CacheTypeDef, "ExecutionRunConnection">;
                    args: {
                        first?: number | null | undefined;
                        after?: string | null | undefined;
                        last?: number | null | undefined;
                        before?: string | null | undefined;
                        artifactID?: string | null | undefined;
                        definitionID?: string | null | undefined;
                    };
                };
                execRun: {
                    type: Record<CacheTypeDef, "ExecutionRun"> | null;
                    args: {
                        id: string;
                    };
                };
                execRunLog: {
                    type: Record<CacheTypeDef, "ExecutionRunLog">;
                    args: {
                        runID: string;
                    };
                };
                health: {
                    type: Record<CacheTypeDef, "HealthStatus">;
                    args: never;
                };
                ready: {
                    type: Record<CacheTypeDef, "ReadyStatus">;
                    args: never;
                };
                coordinators: {
                    type: (Record<CacheTypeDef, "CoordinatorMetricsEntry">)[];
                    args: never;
                };
                coordinatorMetricsByProject: {
                    type: Record<CacheTypeDef, "CoordinatorMetrics"> | null;
                    args: {
                        projectRoot: string;
                    };
                };
                metricsSummary: {
                    type: Record<CacheTypeDef, "AggregateSummary">;
                    args: {
                        since?: string | null | undefined;
                    };
                };
                metricsCost: {
                    type: Record<CacheTypeDef, "CostReport">;
                    args: {
                        since?: string | null | undefined;
                        bead?: string | null | undefined;
                        feature?: string | null | undefined;
                    };
                };
                metricsCycleTime: {
                    type: Record<CacheTypeDef, "CycleTimeReport">;
                    args: {
                        since?: string | null | undefined;
                    };
                };
                metricsRework: {
                    type: Record<CacheTypeDef, "ReworkReport">;
                    args: {
                        since?: string | null | undefined;
                    };
                };
                providers: {
                    type: (Record<CacheTypeDef, "Provider">)[];
                    args: never;
                };
                provider: {
                    type: Record<CacheTypeDef, "Provider"> | null;
                    args: {
                        name: string;
                    };
                };
            };
            fragments: [];
        };
        WorkerEvent: {
            idFields: never;
            fields: {
                eventID: {
                    type: string;
                    args: never;
                };
                workerID: {
                    type: string;
                    args: never;
                };
                phase: {
                    type: string;
                    args: never;
                };
                timestamp: {
                    type: any;
                    args: never;
                };
                logLine: {
                    type: string | null;
                    args: never;
                };
                beadID: {
                    type: string | null;
                    args: never;
                };
            };
            fragments: [];
        };
        BeadEvent: {
            idFields: never;
            fields: {
                eventID: {
                    type: string;
                    args: never;
                };
                beadID: {
                    type: string;
                    args: never;
                };
                kind: {
                    type: string;
                    args: never;
                };
                summary: {
                    type: string | null;
                    args: never;
                };
                body: {
                    type: string | null;
                    args: never;
                };
                actor: {
                    type: string | null;
                    args: never;
                };
                timestamp: {
                    type: any;
                    args: never;
                };
            };
            fragments: [];
        };
        ExecutionEvent: {
            idFields: never;
            fields: {
                eventID: {
                    type: string;
                    args: never;
                };
                runID: {
                    type: string;
                    args: never;
                };
                stream: {
                    type: string;
                    args: never;
                };
                line: {
                    type: string;
                    args: never;
                };
                timestamp: {
                    type: any;
                    args: never;
                };
            };
            fragments: [];
        };
        CoordinatorMetricsUpdate: {
            idFields: never;
            fields: {
                updateID: {
                    type: string;
                    args: never;
                };
                projectRoot: {
                    type: string;
                    args: never;
                };
                timestamp: {
                    type: any;
                    args: never;
                };
                landed: {
                    type: number;
                    args: never;
                };
                preserved: {
                    type: number;
                    args: never;
                };
                failed: {
                    type: number;
                    args: never;
                };
                pushFailed: {
                    type: number;
                    args: never;
                };
                totalDurationMs: {
                    type: number;
                    args: never;
                };
                totalCommits: {
                    type: number;
                    args: never;
                };
            };
            fragments: [];
        };
    };
    lists: {};
    queries: [[TestTypenameStore, TestTypename$result, TestTypename$input], [NodeInfoStore, NodeInfo$result, NodeInfo$input]];
};