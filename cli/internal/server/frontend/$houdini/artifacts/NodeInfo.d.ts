export type NodeInfo = {
    readonly "input": NodeInfo$input;
    readonly "result": NodeInfo$result | undefined;
};

export type NodeInfo$result = {
    /**
     * Return the server node information (maps to GET /api/node)
    */
    readonly nodeInfo: {
        /**
         * Unique node identifier (stable across restarts)
        */
        readonly id: string;
        /**
         * Human-readable node name
        */
        readonly name: string;
    };
};

export type NodeInfo$input = null;

export type NodeInfo$artifact = {
    "name": "NodeInfo";
    "kind": "HoudiniQuery";
    "hash": "e917120d307a10936ea51038bfd5989f947d41deba1c6cf24f8e05a2703bee94";
    "raw": `query NodeInfo {
  nodeInfo {
    id
    name
  }
}
`;
    "rootType": "Query";
    "stripVariables": [];
    "selection": {
        "fields": {
            "nodeInfo": {
                "type": "NodeInfo";
                "keyRaw": "nodeInfo";
                "selection": {
                    "fields": {
                        "id": {
                            "type": "ID";
                            "keyRaw": "id";
                            "visible": true;
                        };
                        "name": {
                            "type": "String";
                            "keyRaw": "name";
                            "visible": true;
                        };
                    };
                };
                "visible": true;
            };
        };
    };
    "pluginData": {
        "houdini-svelte": {};
    };
    "policy": "CacheOrNetwork";
    "partial": false;
};