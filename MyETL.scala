import org.apache.spark.sql.SQLContext
import org.apache.spark.{SparkConf, SparkContext}
 
import scala.xml.XML
 
object MyETL {
 
  def main(args: Array[String]) {
    val inputPath = "123.xml"
 
    val conf = new SparkConf().setAppName("MyETL")
    val sc = new SparkContext(conf)
    val sqlContext = new SQLContext(sc)
 
 
    val pathRDD = sc.wholeTextFiles(inputPath)
    val files = pathRDD.map { case (filename, content) => (filename.substring(56,62), content)}
 
    val xml10pattern = "[^"+ "\u0009\r\n" + "\u0020-\uD7FF" + "\uE000-\uFFFD" + "\ud800\udc00-\udbff\udfff" + "]"
    val xml11pattern = "[^" + "\u0001-\uD7FF" + "\uE000-\uFFFD" + "\ud800\udc00-\udbff\udfff" + "]+"
 
 
    val texts = files.map{s =>
      val asd = s._1.toString
      val xml = XML.loadString(s._2.toString.replaceAll(xml11pattern, ""))
      val dfg = xml \\ "item"
      val rankData = dfg.map(
        xmlNode => (
          (xmlNode \ "abc").text.toString.reverse.padTo(11,"0").reverse.mkString.concat("-").concat((xmlNode \ "bcd").text.toString.reverse.padTo(11,"0").reverse.mkString.concat("-1")),
          asd,
          (xmlNode \ "cde").text.toInt,
          (xmlNode \ "def").text.toString.split('/').indexOf(efg) - 1
        )) filter(_._3 <= 20)
      rankData.toList
    } flatMap(x => x) map{ case (a, b, c, d) => (a, (b, c, d))}
 
    val rankingRDD = texts.aggregateByKey(List[Any]())(
      (aggr, value) => aggr ::: (value :: Nil),
      (aggr1, aggr2) => aggr1 ::: aggr2
    )
 
    def toJson(data:Tuple2[String, List[Any]]):String = {
      val fgh = "$fgh"
      val ghi = data._2.map{ v =>
        v match {
          case (a,b,c) => s"""MySecretNumber"""
        }
      }.mkString(",")
 
      var res = s"""
       {
         "tyu":"${data._1}",
         "fgh":[ghj]
       },
      """
      res
    }
 
    rankingRDD.map(toJson).saveAsTextFile("MySecretJson")
 
  }
}