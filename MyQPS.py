import logging
import pandas as pd
import numpy as np
import matplotlib
import json
matplotlib.use('Agg')
import matplotlib.pyplot as plt

from fbprophet import Prophet
from datetime import date,timedelta


@forecast.route('/data/')
def data():
    maxqps = kpi.find_maxqps()
    avgqps = kpi.find_avgqps()
    totaldoc = kpi.find_totaldoc()
    return json.dumps(maxqps)



@forecast.route('/show/')
def show(chartID = 'chart_ID', chart_type = 'line', chart_height = 400):
    """
    Forecast QPS
    """
    plt.rcParams['figure.figsize'] = (20, 5)
    plt.style.use('ggplot')

    maxqps = kpi.find_maxqps()
    avgqps = kpi.find_avgqps()
    totaldoc = kpi.find_totaldoc()

    title1 = {"text": 'Trend QPS'}
    chart1 = {"renderTo": chartID, "type": chart_type, "height": chart_height, }
    xAxis = {"categories": ['date']}
    yAxis = {"title": {"text": 'My Service'}}
    series1 = [{"name": 'MAX QPS', "data": maxqps}, {"name": 'AVG QPS', "data": avgqps}]


    print("Loading the model...")
    qps_df = pd.read_csv('./qps.csv', index_col='date', parse_dates=True)
    df = qps_df.reset_index()
    df = df.rename(columns={'date': 'ds', 'maxqps': 'y'})
    df['y'] = np.log(df['y'])

    qpsmodel = Prophet(weekly_seasonality=4, seasonality_prior_scale=10)
    qpsmodel.fit(df);

    future = qpsmodel.make_future_dataframe(periods=50, freq='W')
    forecast = qpsmodel.predict(future)

    qpsmodel.plot(forecast);

    df.set_index('ds', inplace=True)
    forecast.set_index('ds', inplace=True)
    viz_df = qps_df.join(forecast[['yhat', 'yhat_lower', 'yhat_upper']], how='outer')
    qps_df.index = pd.to_datetime(qps_df.index)
    last_date = qps_df.index[-1]
    viz_df['yhat_rescaled'] = np.exp(viz_df['yhat'])
    qps_df.index = pd.to_datetime(qps_df.index) 
    connect_date = qps_df.index[-12]  
    connect_date = connect_date - timedelta(weeks=12)
    mask = (forecast.index > connect_date)
    predict_df = forecast.loc[mask]
    viz_df = qps_df.join(predict_df[['yhat', 'yhat_lower', 'yhat_upper']], how='outer')
    viz_df['yhat_scaled'] = np.exp(viz_df['yhat'])
    fig, ax1 = plt.subplots()
    ax1.plot(viz_df.maxqps)
    ax1.plot(viz_df.yhat_scaled, color='black', linestyle=':')
    ax1.fill_between(viz_df.index, np.exp(viz_df['yhat_upper']), np.exp(viz_df['yhat_lower']), alpha=0.5, color='darkgray')
    ax1.set_title('QPS (Orange) vs QPS Forecast (Black)')
    ax1.set_ylabel('MAX QPS')
    ax1.set_xlabel('Date')

    L = ax1.legend()
    L.get_texts()[0].set_text('Actual QPS') 
    L.get_texts()[1].set_text('Forecasted QPS')
    print("The model has been loaded...doing predictions now...")

    from io import BytesIO
    buff = BytesIO()
    plt.savefig(buff, format='png')
    buff.seek(0)
    import base64
    figdata_png = buff.getvalue()
    figdata_png = base64.b64encode(figdata_png)

    return render_template("forecast.html", chartID=chartID, chart1=chart1, series1=series1, title1=title1, xAxis=xAxis, yAxis=yAxis, figdata_png=figdata_png)