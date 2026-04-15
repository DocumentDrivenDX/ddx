export default {
    "name": "NodeInfo",
    "kind": "HoudiniQuery",
    "hash": "e917120d307a10936ea51038bfd5989f947d41deba1c6cf24f8e05a2703bee9",

    "raw": `query NodeInfo {
  nodeInfo {
    id
    name
  }
}
`,

    "rootType": "Query",
    "stripVariables": [],

    "selection": {
        "fields": {
            "nodeInfo": {
                "type": "NodeInfo",
                "keyRaw": "nodeInfo",
                "visible": true,
                "selection": {
                    "fields": {
                        "id": {
                            "type": "ID",
                            "keyRaw": "id",
                            "visible": true
                        },
                        "name": {
                            "type": "String",
                            "keyRaw": "name",
                            "visible": true
                        }
                    }
                }
            }
        }
    },

    "pluginData": {
        "houdini-svelte": {}
    },

    "policy": "CacheOrNetwork",
    "partial": false
};

"HoudiniHash=e917120d307a10936ea51038bfd5989f947d41deba1c6cf24f8e05a2703bee94";
