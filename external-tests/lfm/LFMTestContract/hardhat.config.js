require('@nomiclabs/hardhat-ethers');
require("@nomicfoundation/hardhat-toolbox");
require('@openzeppelin/hardhat-upgrades');

const { PrivateKey } = require('./hardhat/secret.json'); //zzzz set private key to  that of FROM account 

module.exports = {
   defaultNetwork: 'hardhat',

   networks: {
      privatenet: {
         url: 'http://127.0.0.1:33389',  // port == --http.port or 3334 33389(=miner)
         accounts: [PrivateKey],
         chainId: 1116,
      },
      testnet: {
         url: 'https://rpc.test.btcs.network',
         accounts: [PrivateKey],
         chainId: 1115,
      },
      mainnet: {
        url: 'https://rpc.coredao.org',
        accounts: [PrivateKey],
        chainId: 1116,
     },
   },

   etherscan: {
      apiKey: {
        mainnet: "e54785d7a16f4023aac1f97e793cf22a",
        testnet: "8d3e0ad82887432495bc109d35a809bf"
      },
      customChains: [
         {
            network: "mainnet",
            chainId: 1116,
            urls: {
              apiURL: "https://openapi.coredao.org/api",
              browserURL: "https://scan.coredao.org/"
            }
          },
          {
            network: "testnet",
            chainId: 1115,
            urls: {
              apiURL: "https://api.test.btcs.network/api",
              browserURL: "https://scan.test.btcs.network/"
            }
          }
          ]
    },

   solidity: {
      compilers: [
        {
           version: '0.8.20',
           settings: {
              optimizer: {
                 enabled: true,
                 runs: 200,
              },
           },
        },
      ],
   },
   paths: {
      sources: './src',
      cache: './cache',
      artifacts: './artifacts',
   },
   mocha: {
      timeout: 20000,
   },
};
