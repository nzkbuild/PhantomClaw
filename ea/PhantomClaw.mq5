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
input string BridgeAuthToken = "";                    // Optional token sent in X-Phantom-Bridge-Token
input string BridgeContractVersion = "v3";            // Sent in X-Phantom-Bridge-Contract
input int    SignalIntervalSec = 60;                  // Seconds between signal pushes
input int    RequestTimeoutMs  = 500;                 // Short timeout is fine (async ACK design)

//--- Global variables
datetime g_lastSignalTime = 0;
int      g_requestTimeout = 500;
long     g_requestSeq = 0;
string   g_lastRequestID = "";
datetime g_lastOpsPollTime = 0;
string   g_opsOverallStatus = "UNKNOWN";
string   g_opsReasonCode = "";
string   g_opsEALinkStatus = "UNKNOWN";
string   g_opsAuthStatus = "UNKNOWN";
string   g_opsContractStatus = "UNKNOWN";
string   g_opsDecisionStatus = "UNKNOWN";
string   g_opsAIStatus = "UNKNOWN";
int      g_opsLastSignalAgeSec = -1;
int      g_opsQueueDepth = -1;
int      g_opsAuthFailures5m = -1;
int      g_lastHTTPStatus = 0;
string   g_lastHTTPEndpoint = "";
string   g_lastHTTPErrorClass = "";
datetime g_lastDecisionTime = 0;
string   g_lastDecisionAction = "-";

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
   PollOpsHealth();
   RenderStatusPanel();

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
   g_lastHTTPEndpoint = endpoint;
   string headers = "Content-Type: application/json\r\n";
   if(BridgeAuthToken != "")
      headers += "X-Phantom-Bridge-Token: " + BridgeAuthToken + "\r\n";
   if(BridgeContractVersion != "")
      headers += "X-Phantom-Bridge-Contract: " + BridgeContractVersion + "\r\n";
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
      g_lastHTTPStatus = -1;
      g_lastHTTPErrorClass = "network";
      if(err == 4014)
         Print("PhantomClaw: Add ", BridgeHost, " to Tools > Options > Expert Advisors > Allow WebRequest for listed URL");
      else
         Print("PhantomClaw: WebRequest error ", err);
      return "";
   }

   g_lastHTTPStatus = res;
   g_lastHTTPErrorClass = "";
   string body = CharArrayToString(result, 0, WHOLE_ARRAY, CP_UTF8);
   if(res == 401)
      g_lastHTTPErrorClass = "unauthorized";
   else if(res == 400 && StringFind(body, "incompatible contract version") >= 0)
      g_lastHTTPErrorClass = "contract_mismatch";
   else if(res >= 500)
      g_lastHTTPErrorClass = "server_error";
   else if(res >= 400)
      g_lastHTTPErrorClass = "client_error";

   return body;
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

void PollOpsHealth()
{
   datetime now = TimeCurrent();
   if(now - g_lastOpsPollTime < 5) return;
   g_lastOpsPollTime = now;

   string response = GetFromAgent("/health/ops");
   if(response == "") return;

   g_opsOverallStatus = ExtractJSONString(response, "overall_status");
   g_opsReasonCode = ExtractJSONString(response, "overall_reason_code");
   g_opsEALinkStatus = ExtractJSONString(response, "ea_link_status");
   g_opsAuthStatus = ExtractJSONString(response, "bridge_auth_status");
   g_opsContractStatus = ExtractJSONString(response, "contract_compat_status");
   g_opsDecisionStatus = ExtractJSONString(response, "decision_loop_status");
   g_opsAIStatus = ExtractJSONString(response, "ai_health_status");
   g_opsLastSignalAgeSec = (int)ExtractJSONDouble(response, "last_signal_age_sec");
   g_opsQueueDepth = (int)ExtractJSONDouble(response, "queue_depth_active");
   g_opsAuthFailures5m = (int)ExtractJSONDouble(response, "auth_failures_5m");
}

void RenderStatusPanel()
{
   string status = g_opsOverallStatus;
   if(status == "") status = "UNKNOWN";
   string reason = g_opsReasonCode;
   if(reason == "") reason = "n/a";

   string signalAge = "n/a";
   if(g_opsLastSignalAgeSec >= 0)
   {
      if(g_opsLastSignalAgeSec >= 3600)
         signalAge = IntegerToString(g_opsLastSignalAgeSec / 3600) + "h";
      else if(g_opsLastSignalAgeSec >= 60)
         signalAge = IntegerToString(g_opsLastSignalAgeSec / 60) + "m";
      else
         signalAge = IntegerToString(g_opsLastSignalAgeSec) + "s";
   }

   string decisionAge = "n/a";
   if(g_lastDecisionTime > 0)
      decisionAge = IntegerToString((int)(TimeCurrent() - g_lastDecisionTime)) + "s";

   string txt = "PhantomClaw EA Bridge\n";
   txt += "Overall: " + status + " (" + reason + ")\n";
   txt += "EA Link/Auth/Contract: " + g_opsEALinkStatus + " / " + g_opsAuthStatus + " / " + g_opsContractStatus + "\n";
   txt += "AI/Decision: " + g_opsAIStatus + " / " + g_opsDecisionStatus + " | Queue=" + IntegerToString(g_opsQueueDepth) + "\n";
   txt += "Last Signal ReqID: " + g_lastRequestID + " | Signal Age: " + signalAge + "\n";
   txt += "Last Decision: " + g_lastDecisionAction + " (" + decisionAge + " ago)\n";
   txt += "HTTP " + IntegerToString(g_lastHTTPStatus) + " @ " + g_lastHTTPEndpoint;
   if(g_lastHTTPErrorClass != "")
      txt += " [" + g_lastHTTPErrorClass + "]";
   txt += "\nAuth failures (5m): " + IntegerToString(g_opsAuthFailures5m);
   Comment(txt);
}

//+------------------------------------------------------------------+
//| Process agent response — execute pending order actions            |
//+------------------------------------------------------------------+
void ProcessResponse(string response)
{
   if(StringLen(response) == 0) return;

   // Parse action field
   string action = ExtractJSONString(response, "action");
   if(action == "")
   {
      Print("PhantomClaw: invalid response payload (missing action): ", response);
      return;
   }
   g_lastDecisionAction = action;
   g_lastDecisionTime = TimeCurrent();

   if(action == "HOLD") return;

   if(action == "PLACE_PENDING")
   {
      string type   = ExtractJSONString(response, "type");
      string symbol = ExtractJSONString(response, "symbol");
      double level  = ExtractJSONDouble(response, "level");
      double lot    = ExtractJSONDouble(response, "lot");
      double sl     = ExtractJSONDouble(response, "sl");
      double tp     = ExtractJSONDouble(response, "tp");

      if(symbol == "" || level <= 0.0 || lot <= 0.0)
      {
         Print("PhantomClaw: invalid PLACE_PENDING response (symbol/level/lot): ", response);
         return;
      }

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
      if(ticket <= 0)
      {
         Print("PhantomClaw: invalid CANCEL_PENDING response (ticket): ", response);
         return;
      }
      CancelPendingOrder(ticket);
   }
   else if(action == "MODIFY_PENDING")
   {
      long   ticket = (long)ExtractJSONDouble(response, "ticket");
      double newSL  = ExtractJSONDouble(response, "sl");
      double newTP  = ExtractJSONDouble(response, "tp");
      if(ticket <= 0)
      {
         Print("PhantomClaw: invalid MODIFY_PENDING response (ticket): ", response);
         return;
      }
      ModifyPendingOrder(ticket, newSL, newTP);
   }
   else if(action == "MARKET_CLOSE")
   {
      long ticket = (long)ExtractJSONDouble(response, "ticket");
      if(ticket <= 0)
      {
         Print("PhantomClaw: invalid MARKET_CLOSE response (ticket): ", response);
         return;
      }
      ClosePosition(ticket);
   }
   else
   {
      Print("PhantomClaw: unknown action: ", action, " payload=", response);
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
//| Resolve weighted entry price for a closed position               |
//+------------------------------------------------------------------+
double ResolveEntryPrice(const ulong positionId, const string symbol)
{
   if(positionId == 0) return 0.0;
   if(!HistorySelect(0, TimeCurrent())) return 0.0;

   double weightedPrice = 0.0;
   double totalVolume = 0.0;
   int deals = HistoryDealsTotal();

   for(int i = 0; i < deals; i++)
   {
      ulong dealTicket = HistoryDealGetTicket(i);
      if(dealTicket == 0) continue;

      if((ulong)HistoryDealGetInteger(dealTicket, DEAL_POSITION_ID) != positionId) continue;
      if(HistoryDealGetString(dealTicket, DEAL_SYMBOL) != symbol) continue;

      long dealEntry = HistoryDealGetInteger(dealTicket, DEAL_ENTRY);
      if(dealEntry != DEAL_ENTRY_IN && dealEntry != DEAL_ENTRY_INOUT) continue;

      double dealVolume = HistoryDealGetDouble(dealTicket, DEAL_VOLUME);
      double dealPrice = HistoryDealGetDouble(dealTicket, DEAL_PRICE);
      if(dealVolume <= 0.0 || dealPrice <= 0.0) continue;

      weightedPrice += dealPrice * dealVolume;
      totalVolume += dealVolume;
   }

   if(totalVolume <= 0.0) return 0.0;
   return weightedPrice / totalVolume;
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
   ulong  positionId = (ulong)HistoryDealGetInteger(trans.deal, DEAL_POSITION_ID);

   // Only report closes (DEAL_ENTRY_OUT)
   if(entry != DEAL_ENTRY_OUT) return;
   double entryPrice = ResolveEntryPrice(positionId, symbol);
   if(entryPrice <= 0.0)
   {
      Print("PhantomClaw: Could not resolve entry price for position ", positionId, " on ", symbol);
      return;
   }

   string json = "{";
   json += "\"ticket\":" + IntegerToString(trans.deal) + ",";
   json += "\"symbol\":\"" + symbol + "\",";
   json += "\"direction\":\"" + ((HistoryDealGetInteger(trans.deal, DEAL_TYPE) == DEAL_TYPE_BUY) ? "SELL" : "BUY") + "\",";
   json += "\"entry\":" + DoubleToString(entryPrice, 5) + ",";
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
   int valuePos = FindJSONValueStart(json, key);
   if(valuePos < 0) return "";

   int len = StringLen(json);
   if(valuePos >= len || StringSubstr(json, valuePos, 1) != "\"") return "";
   int i = valuePos + 1;
   string out = "";
   bool escaped = false;
   while(i < len)
   {
      string ch = StringSubstr(json, i, 1);
      if(escaped)
      {
         out += ch;
         escaped = false;
      }
      else if(ch == "\\")
      {
         escaped = true;
      }
      else if(ch == "\"")
      {
         return out;
      }
      else
      {
         out += ch;
      }
      i++;
   }
   return "";
}

double ExtractJSONDouble(string json, string key)
{
   int start = FindJSONValueStart(json, key);
   if(start < 0) return 0;

   int len = StringLen(json);
   int i = start;

   // Optional quote handling for numeric strings.
   bool quoted = false;
   if(i < len && StringSubstr(json, i, 1) == "\"")
   {
      quoted = true;
      i++;
   }

   string num = "";
   while(i < len)
   {
      string ch = StringSubstr(json, i, 1);
      bool isDigit = (ch >= "0" && ch <= "9");
      if(isDigit || ch == "-" || ch == "+" || ch == "." || ch == "e" || ch == "E")
      {
         num += ch;
         i++;
         continue;
      }
      if(quoted && ch == "\"") break;
      if(!quoted && (ch == "," || ch == "}" || ch == "]")) break;
      if(ch == " " || ch == "\t" || ch == "\r" || ch == "\n")
      {
         if(StringLen(num) > 0) break;
         i++;
         continue;
      }
      break;
   }

   if(StringLen(num) == 0) return 0;
   return StringToDouble(num);
}

int FindJSONValueStart(string json, string key)
{
   string search = "\"" + key + "\"";
   int keyPos = StringFind(json, search);
   if(keyPos < 0) return -1;

   int i = keyPos + StringLen(search);
   int len = StringLen(json);
   while(i < len)
   {
      string ch = StringSubstr(json, i, 1);
      if(ch == ":")
      {
         i++;
         break;
      }
      if(ch == " " || ch == "\t" || ch == "\r" || ch == "\n")
      {
         i++;
         continue;
      }
      return -1;
   }
   while(i < len)
   {
      string ch = StringSubstr(json, i, 1);
      if(ch == " " || ch == "\t" || ch == "\r" || ch == "\n")
      {
         i++;
         continue;
      }
      return i;
   }
   return -1;
}
//+------------------------------------------------------------------+
