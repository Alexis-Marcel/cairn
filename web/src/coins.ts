// Coins to pick from when entering an operation: the common ones, by
// market size. Any other symbol can be typed.

export interface CoinInfo {
  symbol: string;
  name: string;
}

export const COINS: CoinInfo[] = [
  { symbol: "BTC", name: "Bitcoin" },
  { symbol: "ETH", name: "Ethereum" },
  { symbol: "USDT", name: "Tether" },
  { symbol: "USDC", name: "USD Coin" },
  { symbol: "EURC", name: "Euro Coin" },
  { symbol: "BNB", name: "BNB" },
  { symbol: "SOL", name: "Solana" },
  { symbol: "XRP", name: "XRP" },
  { symbol: "ADA", name: "Cardano" },
  { symbol: "DOGE", name: "Dogecoin" },
  { symbol: "TRX", name: "TRON" },
  { symbol: "AVAX", name: "Avalanche" },
  { symbol: "DOT", name: "Polkadot" },
  { symbol: "LINK", name: "Chainlink" },
  { symbol: "TON", name: "Toncoin" },
  { symbol: "POL", name: "Polygon" },
  { symbol: "MATIC", name: "Polygon (ancien)" },
  { symbol: "LTC", name: "Litecoin" },
  { symbol: "BCH", name: "Bitcoin Cash" },
  { symbol: "SHIB", name: "Shiba Inu" },
  { symbol: "DAI", name: "Dai" },
  { symbol: "UNI", name: "Uniswap" },
  { symbol: "XLM", name: "Stellar" },
  { symbol: "ATOM", name: "Cosmos" },
  { symbol: "XMR", name: "Monero" },
  { symbol: "ETC", name: "Ethereum Classic" },
  { symbol: "NEAR", name: "NEAR" },
  { symbol: "APT", name: "Aptos" },
  { symbol: "SUI", name: "Sui" },
  { symbol: "ARB", name: "Arbitrum" },
  { symbol: "OP", name: "Optimism" },
  { symbol: "ICP", name: "Internet Computer" },
  { symbol: "FIL", name: "Filecoin" },
  { symbol: "HBAR", name: "Hedera" },
  { symbol: "VET", name: "VeChain" },
  { symbol: "ALGO", name: "Algorand" },
  { symbol: "AAVE", name: "Aave" },
  { symbol: "MKR", name: "Maker" },
  { symbol: "GRT", name: "The Graph" },
  { symbol: "INJ", name: "Injective" },
  { symbol: "RNDR", name: "Render" },
  { symbol: "TAO", name: "Bittensor" },
  { symbol: "PEPE", name: "Pepe" },
  { symbol: "XTZ", name: "Tezos" },
  { symbol: "EGLD", name: "MultiversX" },
  { symbol: "SAND", name: "The Sandbox" },
  { symbol: "MANA", name: "Decentraland" },
  { symbol: "CRV", name: "Curve" },
  { symbol: "COMP", name: "Compound" },
  { symbol: "KAS", name: "Kaspa" },
  { symbol: "SEI", name: "Sei" },
  { symbol: "TIA", name: "Celestia" },
];

/** Euros, for deposits and withdrawals of cash. */
export const EUR: CoinInfo = { symbol: "EUR", name: "Euro" };
