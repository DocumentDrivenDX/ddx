export default {
    "name": "NodeInfo",
    "kind": "HoudiniQuery",
    "hash": "e917120d307a10936ea51038bfd5989f947d41deba1c6cf24f8e05a2703bee94",

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
                },

                "visible": true
            }
        }
    },

    "pluginData": {
        "houdini-svelte": {}
    },

    "policy": "CacheOrNetwork",
    "partial": false
};

"HoudiniHash=24630227d2f53008bff2da70ee0bdd009cf6265327e88152e4873d6c7f892217";