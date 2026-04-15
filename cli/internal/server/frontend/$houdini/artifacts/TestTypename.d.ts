export type TestTypename = {
    readonly "input": TestTypename$input;
    readonly "result": TestTypename$result | undefined;
};

export type TestTypename$result = {
    readonly __typename: string | null;
};

export type TestTypename$input = null;

export type TestTypename$artifact = {
    "name": "TestTypename";
    "kind": "HoudiniQuery";
    "hash": "a793a984282f3bec7d0a5472a98d0d90d8987497774c34732f250b25b8729727";
    "raw": `query TestTypename {
  __typename
}
`;
    "rootType": "Query";
    "stripVariables": [];
    "selection": {
        "fields": {
            "__typename": {
                "type": "String";
                "keyRaw": "__typename";
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