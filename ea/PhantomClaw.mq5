//+------------------------------------------------------------------+
//| PhantomClaw.mq5 - AI Trading Agent EA Bridge                      |
//| Bridges MT5 with PhantomClaw Go agent via REST API                |
//| PRD v1.0 — Pending order execution model                         |
//+------------------------------------------------------------------+
#property copyright "PhantomClaw"
#property link      "https://github.com/nzkbuild/PhantomClaw"
#property version   "1.00"
#property strict

//--- Input parameters
input string BridgeHost = "http://127.0.0.1:8765";  // Go agent REST endpoint
input int    SignalIntervalSec = 60;                  // Seconds between signal pushes
input int    RequestTimeoutMs  = 500;                 // Short timeout is fine (async ACK design)

//--- Global variables
datetime g_lastSignalTime = 0;
int      g_requestTimeout = 500;
long     g_requestSeq = 0;
string   g_lastRequestID = "";

//+------------------------------------------------------------------+
//| Expert initialization                                             |
//+------------------------------------------------------------------+
int OnInit()
{
   g_requestTimeout = RequestTimeoutMs;
   Print("PhantomClaw EA initialized — bridge: ", BridgeHost);
   Print("Signal interval: ", SignalIntervalSec, "s | Timeout: ", g_requestTimeout, "ms");
   return(INIT_SUCCEEDED);
}

//+------------------------------------------------------------------+
//| Expert deinitialization                                           |
//+------------------------------------------------------------------+
void OnDeinit(const int reason)
{
   Print("PhantomClaw EA stopped. Reason: ", reason);
}

//+------------------------------------------------------------------+
//| Expert tick function — pushes signal on interval                  |
//+------------------------------------------------------------------+
void OnTick()
{
   // 1) Poll and execute latest async decision (if any).
   PollDecision();

   // 2) Push fresh signal on cadence (fire-and-forget).
   datetime now = TimeCurrent();
   if(now - g_lastSignalTime < SignalIntervalSec) return;
   g_lastSignalTime = now;

   // Build and send signal payload.
   string payload = BuildSignalPayload();
   PostToAgent("/signal", payload);
}

//+------------------------------------------------------------------+
//| Build JSON signal payload with MTF OHLCV + indicators            |
//+------------------------------------------------------------------+
string BuildSignalPayload()
{
   string symbol = Symbol();
   g_requestSeq++;
   g_lastRequestID = symbol + "_" + IntegerToString((int)TimeCurrent()) + "_" + IntegerToString((int)g_requestSeq);
   double bid = SymbolInfoDouble(symbol, SYMBOL_BID);
   double ask = SymbolInfoDouble(symbol, SYMBOL_ASK);
   double spread = (ask - bid) / SymbolInfoDouble(symbol, SYMBOL_POINT);
   double balance = AccountInfoDouble(ACCOUNT_BALANCE);
   double equity = AccountInfoDouble(ACCOUNT_EQUITY);
   double margin = AccountInfoDouble(ACCOUNT_MARGIN);
   double freeMargin = AccountInfoDouble(ACCOUNT_MARGIN_FREE);
   int openPositions = PositionsTotal();

   // Current candle data (H1)
   double o = iOpen(symbol, PERIOD_H1, 0);
   double h = iHigh(symbol, PERIOD_H1, 0);
   double l = iLow(symbol, PERIOD_H1, 0);
   double c = iClose(symbol, PERIOD_H1, 0);
   long   v = iVolume(symbol, PERIOD_H1, 0);

   // RSI 14 on H1
   int rsiHandle = iRSI(symbol, PERIOD_H1, 14, PRICE_CLOSE);
   double rsiVal[];
   ArraySetAsSeries(rsiVal, true);
   CopyBuffer(rsiHandle, 0, 0, 1, rsiVal);
   double rsi = (ArraySize(rsiVal) > 0) ? rsiVal[0] : 0;
   IndicatorRelease(rsiHandle);

   // ATR 14 on H1
   int atrHandle = iATR(symbol, PERIOD_H1, 14);
   double atrVal[];
   ArraySetAsSeries(atrVal, true);
   CopyBuffer(atrHandle, 0, 0, 1, atrVal);
   double atr = (ArraySize(atrVal) > 0) ? atrVal[0] : 0;
   IndicatorRelease(atrHandle);

   // Build JSON manually (MQL5 has no JSON library)
   string json = "{";
   json += "\"request_id\":\"" + g_lastRequestID + "\",";
   json += "\"symbol\":\"" + symbol + "\",";
   json += "\"timeframe\":\"H1\",";
   json += "\"bid\":" + DoubleToString(bid, 5) + ",";
   json += "\"ask\":" + DoubleToString(ask, 5) + ",";
   json += "\"spread\":" + DoubleToString(spread, 1) + ",";
   json += "\"balance\":" + DoubleToString(balance, 2) + ",";
   json += "\"equity\":" + DoubleToString(equity, 2) + ",";
   json += "\"margin\":" + DoubleToString(margin, 2) + ",";
   json += "\"free_margin\":" + DoubleToString(freeMargin, 2) + ",";
   json += "\"open_positions\":" + IntegerToString(openPositions) + ",";
   json += "\"ohlcv\":{\"H1\":[{";
   json += "\"o\":" + DoubleToString(o, 5) + ",";
   json += "\"h\":" + DoubleToString(h, 5) + ",";
   json += "\"l\":" + DoubleToString(l, 5) + ",";
   json += "\"c\":" + DoubleToString(c, 5) + ",";
   json += "\"v\":" + IntegerToString(v) + "}]},";
   json += "\"indicators\":{";
   json += "\"rsi_14\":" + DoubleToString(rsi, 2) + ",";
   json += "\"atr_14\":" + DoubleToString(atr, 6) + "},";
   json += "\"timestamp\":\"" + TimeToString(TimeCurrent(), TIME_DATE|TIME_SECONDS) + "\"";
   json += "}";
   return json;
}

//+------------------------------------------------------------------+
//| Send POST request to Go agent                                    |
//+------------------------------------------------------------------+
string RequestAgent(string method, string endpoint, string payload)
{
   string url = BridgeHost + endpoint;
   string headers = "Content-Type: application/json\r\n";
   char   postData[];
   char   result[];
   string resultHeaders;

   StringToCharArray(payload, postData, 0, WHOLE_ARRAY, CP_UTF8);
   // Remove null terminator added by StringToCharArray
   if(ArraySize(postData) > 0)
      ArrayResize(postData, ArraySize(postData) - 1);

   int res = WebRequest(
      method,
      url,
      headers,
      g_requestTimeout,
      postData,
      result,
      resultHeaders
   );

   if(res == -1)
   {
      int err = GetLastError();
      if(err == 4014)
         Print("PhantomClaw: Add ", BridgeHost, " to Tools > Options > Expert Advisors > Allow WebRequest for listed URL");
      else
         Print("PhantomClaw: WebRequest error ", err);
      return "";
   }

   return CharArrayToString(result, 0, WHOLE_ARRAY, CP_UTF8);
}

string PostToAgent(string endpoint, string payload)
{
   return RequestAgent("POST", endpoint, payload);
}

string GetFromAgent(string endpoint)
{
   return RequestAgent("GET", endpoint, "");
}

void PollDecision()
{
   string symbol = Symbol();
   string endpoint = "/decision?symbol=" + symbol + "&consume=1";
   if(g_lastRequestID != "")
      endpoint += "&request_id=" + g_lastRequestID;
   string response = GetFromAgent(endpoint);
   if(response == "") return;
   ProcessResponse(response);
}

//+------------------------------------------------------------------+
//| Process agent response — execute pending order actions            |
//+------------------------------------------------------------------+
void ProcessResponse(string response)
{
   // Parse action field
   string action = ExtractJSONString(response, "action");

   if(action == "HOLD") return;

   if(action == "PLACE_PENDING")
   {
      string type   = ExtractJSONString(response, "type");
      string symbol = ExtractJSONString(response, "symbol");
      double level  = ExtractJSONDouble(response, "level");
      double lot    = ExtractJSONDouble(response, "lot");
      double sl     = ExtractJSONDouble(response, "sl");
      double tp     = ExtractJSONDouble(response, "tp");

      ENUM_ORDER_TYPE orderType;
      if(type == "BUY_LIMIT")       orderType = ORDER_TYPE_BUY_LIMIT;
      else if(type == "SELL_LIMIT") orderType = ORDER_TYPE_SELL_LIMIT;
      else if(type == "BUY_STOP")   orderType = ORDER_TYPE_BUY_STOP;
      else if(type == "SELL_STOP")  orderType = ORDER_TYPE_SELL_STOP;
      else { Print("PhantomClaw: unknown order type: ", type); return; }

      PlacePendingOrder(symbol, orderType, lot, level, sl, tp);
   }
   else if(action == "CANCEL_PENDING")
   {
      long ticket = (long)ExtractJSONDouble(response, "ticket");
      CancelPendingOrder(ticket);
   }
   else if(action == "MODIFY_PENDING")
   {
      long   ticket = (long)ExtractJSONDouble(response, "ticket");
      double newSL  = ExtractJSONDouble(response, "sl");
      double newTP  = ExtractJSONDouble(response, "tp");
      ModifyPendingOrder(ticket, newSL, newTP);
   }
   else if(action == "MARKET_CLOSE")
   {
      long ticket = (long)ExtractJSONDouble(response, "ticket");
      ClosePosition(ticket);
   }
}

//+------------------------------------------------------------------+
//| Place a pending order                                            |
//+------------------------------------------------------------------+
void PlacePendingOrder(string symbol, ENUM_ORDER_TYPE type, double lot, double price, double sl, double tp)
{
   MqlTradeRequest request = {};
   MqlTradeResult  result  = {};

   request.action    = TRADE_ACTION_PENDING;
   request.symbol    = symbol;
   request.volume    = lot;
   request.type      = type;
   request.price     = price;
   request.sl        = sl;
   request.tp        = tp;
   request.comment   = "PhantomClaw";
   request.type_time = ORDER_TIME_DAY; // Expires end of day

   if(!OrderSend(request, result))
      Print("PhantomClaw: OrderSend failed — ", result.retcode, " ", result.comment);
   else
      Print("PhantomClaw: Pending order placed — ticket ", result.order, " ", EnumToString(type), " ", symbol, " @ ", price);
}

//+------------------------------------------------------------------+
//| Cancel a pending order by ticket                                 |
//+------------------------------------------------------------------+
void CancelPendingOrder(long ticket)
{
   MqlTradeRequest request = {};
   MqlTradeResult  result  = {};

   request.action = TRADE_ACTION_REMOVE;
   request.order  = (ulong)ticket;

   if(!OrderSend(request, result))
      Print("PhantomClaw: Cancel failed — ", result.retcode, " ", result.comment);
   else
      Print("PhantomClaw: Pending order cancelled — ticket ", ticket);
}

//+------------------------------------------------------------------+
//| Modify SL/TP on a pending order                                  |
//+------------------------------------------------------------------+
void ModifyPendingOrder(long ticket, double sl, double tp)
{
   MqlTradeRequest request = {};
   MqlTradeResult  result  = {};

   request.action = TRADE_ACTION_MODIFY;
   request.order  = (ulong)ticket;
   request.sl     = sl;
   request.tp     = tp;

   if(!OrderSend(request, result))
      Print("PhantomClaw: Modify failed — ", result.retcode, " ", result.comment);
   else
      Print("PhantomClaw: Pending order modified — ticket ", ticket);
}

//+------------------------------------------------------------------+
//| Close an open position (emergency / HALT)                        |
//+------------------------------------------------------------------+
void ClosePosition(long ticket)
{
   MqlTradeRequest request = {};
   MqlTradeResult  result  = {};

   if(!PositionSelectByTicket((ulong)ticket))
   {
      Print("PhantomClaw: Position not found — ticket ", ticket);
      return;
   }

   request.action   = TRADE_ACTION_DEAL;
   request.position = (ulong)ticket;
   request.symbol   = PositionGetString(POSITION_SYMBOL);
   request.volume   = PositionGetDouble(POSITION_VOLUME);
   request.type     = (PositionGetInteger(POSITION_TYPE) == POSITION_TYPE_BUY) ? ORDER_TYPE_SELL : ORDER_TYPE_BUY;
   request.price    = (request.type == ORDER_TYPE_SELL) ?
                      SymbolInfoDouble(request.symbol, SYMBOL_BID) :
                      SymbolInfoDouble(request.symbol, SYMBOL_ASK);
   request.comment  = "PhantomClaw HALT";

   if(!OrderSend(request, result))
      Print("PhantomClaw: Close failed — ", result.retcode, " ", result.comment);
   else
      Print("PhantomClaw: Position closed — ticket ", ticket);
}

//+------------------------------------------------------------------+
//| Trade transaction handler — push results to agent                |
//+------------------------------------------------------------------+
void OnTradeTransaction(
   const MqlTradeTransaction &trans,
   const MqlTradeRequest &request,
   const MqlTradeResult &result)
{
   // Only report deal adds (position opened or closed)
   if(trans.type != TRADE_TRANSACTION_DEAL_ADD) return;

   // Check if this deal has PhantomClaw comment
   if(!HistoryDealSelect(trans.deal)) return;
   string comment = HistoryDealGetString(trans.deal, DEAL_COMMENT);
   if(StringFind(comment, "PhantomClaw") < 0) return;

   // Report to agent
   string symbol = HistoryDealGetString(trans.deal, DEAL_SYMBOL);
   double price  = HistoryDealGetDouble(trans.deal, DEAL_PRICE);
   double volume = HistoryDealGetDouble(trans.deal, DEAL_VOLUME);
   double pnl    = HistoryDealGetDouble(trans.deal, DEAL_PROFIT);
   long   entry  = HistoryDealGetInteger(trans.deal, DEAL_ENTRY);

   // Only report closes (DEAL_ENTRY_OUT)
   if(entry != DEAL_ENTRY_OUT) return;

   string json = "{";
   json += "\"ticket\":" + IntegerToString(trans.deal) + ",";
   json += "\"symbol\":\"" + symbol + "\",";
   json += "\"direction\":\"" + ((HistoryDealGetInteger(trans.deal, DEAL_TYPE) == DEAL_TYPE_BUY) ? "SELL" : "BUY") + "\",";
   json += "\"exit\":" + DoubleToString(price, 5) + ",";
   json += "\"lot\":" + DoubleToString(volume, 2) + ",";
   json += "\"pnl\":" + DoubleToString(pnl, 2) + ",";
   json += "\"closed_at\":\"" + TimeToString(TimeCurrent(), TIME_DATE|TIME_SECONDS) + "\"";
   json += "}";

   PostToAgent("/trade-result", json);
}

//+------------------------------------------------------------------+
//| Simple JSON string extraction (no library in MQL5)               |
//+------------------------------------------------------------------+
string ExtractJSONString(string json, string key)
{
   string search = "\"" + key + "\":\"";
   int start = StringFind(json, search);
   if(start < 0) return "";
   start += StringLen(search);
   int end = StringFind(json, "\"", start);
   if(end < 0) return "";
   return StringSubstr(json, start, end - start);
}

double ExtractJSONDouble(string json, string key)
{
   string search = "\"" + key + "\":";
   int start = StringFind(json, search);
   if(start < 0) return 0;
   start += StringLen(search);
   // Find end: comma, }, or end of string
   int endComma = StringFind(json, ",", start);
   int endBrace = StringFind(json, "}", start);
   int end = (endComma >= 0 && endComma < endBrace) ? endComma : endBrace;
   if(end < 0) return 0;
   return StringToDouble(StringSubstr(json, start, end - start));
}
//+------------------------------------------------------------------+
